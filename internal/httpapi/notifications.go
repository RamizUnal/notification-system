package httpapi

import (
	"net/http"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

const idempotencyHeader = "Idempotency-Key"

// createNotification handles POST /api/v1/notifications.
//
// It answers 202 Accepted: the notification is stored and queued, and
// delivery happens asynchronously. Repeating a request with the same
// Idempotency-Key answers 200 with the original notification and the header
// Idempotent-Replayed: true.
func (a *API) createNotification(w http.ResponseWriter, r *http.Request) error {
	var req notificationRequest
	if err := decodeJSON(w, r, &req, maxSmallBody); err != nil {
		return err
	}

	var verr domain.ValidationError
	in := req.toInput(&verr, "")
	if key := r.Header.Get(idempotencyHeader); key != "" {
		if in.IdempotencyKey != "" && in.IdempotencyKey != key {
			verr.Add("idempotency_key", domain.CodeInvalidValue,
				"idempotency_key differs from the Idempotency-Key header; send one or make them equal")
		}
		in.IdempotencyKey = key
	}
	if err := verr.Err(); err != nil {
		return err
	}

	res, err := a.cfg.Notifications.Create(r.Context(), in)
	if err != nil {
		return err
	}
	w.Header().Set("Location", "/api/v1/notifications/"+res.Notification.ID.String())
	return writeJSON(w, created(w, res.Replayed), toNotificationResponse(res.Notification))
}

// listNotifications handles GET /api/v1/notifications.
func (a *API) listNotifications(w http.ResponseWriter, r *http.Request) error {
	filter, err := parseListFilter(r.URL.Query())
	if err != nil {
		return err
	}
	page, err := a.cfg.Notifications.List(r.Context(), filter)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toListResponse(page))
}

// getNotification handles GET /api/v1/notifications/{id}.
func (a *API) getNotification(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "notification")
	if err != nil {
		return err
	}
	n, err := a.cfg.Notifications.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toNotificationResponse(n))
}

// listAttempts handles GET /api/v1/notifications/{id}/attempts.
func (a *API) listAttempts(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "notification")
	if err != nil {
		return err
	}
	attempts, err := a.cfg.Notifications.Attempts(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, struct {
		Data []attemptResponse `json:"data"`
	}{toAttemptResponses(attempts)})
}

// cancelNotification handles POST /api/v1/notifications/{id}/cancel.
// Cancellation is a state transition, not a deletion: the notification and
// its history remain queryable.
func (a *API) cancelNotification(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "notification")
	if err != nil {
		return err
	}
	n, err := a.cfg.Notifications.Cancel(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toNotificationResponse(n))
}

// retryNotification handles POST /api/v1/notifications/{id}/retry.
func (a *API) retryNotification(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "notification")
	if err != nil {
		return err
	}
	n, err := a.cfg.Notifications.Retry(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toNotificationResponse(n))
}

// createBatch handles POST /api/v1/batches.
func (a *API) createBatch(w http.ResponseWriter, r *http.Request) error {
	var req batchRequest
	if err := decodeJSON(w, r, &req, maxBatchBody); err != nil {
		return err
	}

	var verr domain.ValidationError
	items := make([]service.NotificationInput, len(req.Notifications))
	for i, item := range req.Notifications {
		items[i] = item.toInput(&verr, itemPrefix(i))
	}
	if err := verr.Err(); err != nil {
		return err
	}

	res, err := a.cfg.Notifications.CreateBatch(r.Context(), service.BatchInput{
		Items:          items,
		IdempotencyKey: r.Header.Get(idempotencyHeader),
	})
	if err != nil {
		return err
	}
	w.Header().Set("Location", "/api/v1/batches/"+res.Summary.ID.String())
	return writeJSON(w, created(w, res.Replayed), toBatchResponse(res.Summary, res.NotificationIDs))
}

// getBatch handles GET /api/v1/batches/{id}.
func (a *API) getBatch(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "batch")
	if err != nil {
		return err
	}
	summary, err := a.cfg.Notifications.Batch(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toBatchResponse(summary, nil))
}

// listBatchNotifications handles GET /api/v1/batches/{id}/notifications,
// which accepts the same filters as the notification listing.
func (a *API) listBatchNotifications(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "batch")
	if err != nil {
		return err
	}
	if _, err := a.cfg.Notifications.Batch(r.Context(), id); err != nil {
		return err
	}
	filter, err := parseListFilter(r.URL.Query())
	if err != nil {
		return err
	}
	filter.BatchID = &id
	page, err := a.cfg.Notifications.List(r.Context(), filter)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toListResponse(page))
}

// cancelBatch handles POST /api/v1/batches/{id}/cancel.
func (a *API) cancelBatch(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "batch")
	if err != nil {
		return err
	}
	cancelled, err := a.cfg.Notifications.CancelBatch(r.Context(), id)
	if err != nil {
		return err
	}
	summary, err := a.cfg.Notifications.Batch(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, struct {
		Cancelled int           `json:"cancelled"`
		Batch     batchResponse `json:"batch"`
	}{cancelled, toBatchResponse(summary, nil)})
}

// created picks the status of a create response: 202 for new work, 200 for
// an idempotent replay of an earlier request.
func created(w http.ResponseWriter, replayed bool) int {
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		return http.StatusOK
	}
	return http.StatusAccepted
}

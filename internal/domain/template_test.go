package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
)

func TestTemplateRender(t *testing.T) {
	t.Parallel()

	tpl := domain.Template{
		Name: "order_shipped",
		Body: "Hi {{name}}, order {{ order_id }} ships today. Thanks, {{name}}!",
	}

	t.Run("substitutes every placeholder", func(t *testing.T) {
		t.Parallel()
		got, err := tpl.Render(map[string]string{"name": "Ada", "order_id": "#42", "unused": "x"})
		require.NoError(t, err)
		assert.Equal(t, "Hi Ada, order #42 ships today. Thanks, Ada!", got)
	})

	t.Run("reports each missing variable once", func(t *testing.T) {
		t.Parallel()
		_, err := tpl.Render(map[string]string{})
		var missing *domain.MissingVariablesError
		require.ErrorAs(t, err, &missing)
		assert.Equal(t, []string{"name", "order_id"}, missing.Names)
	})

	t.Run("lists variables in order of first use", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"name", "order_id"}, tpl.Variables())
	})

	t.Run("text without placeholders renders verbatim", func(t *testing.T) {
		t.Parallel()
		plain := domain.Template{Body: "Braces like }} are fine"}
		got, err := plain.Render(nil)
		require.NoError(t, err)
		assert.Equal(t, "Braces like }} are fine", got)
		assert.Empty(t, plain.Variables())
	})
}

func TestTemplateDraftValidate(t *testing.T) {
	t.Parallel()

	valid := domain.TemplateDraft{Name: "welcome-email", Channel: domain.ChannelEmail, Body: "Welcome, {{name}}!"}
	require.NoError(t, valid.Validate())

	tests := []struct {
		name  string
		draft domain.TemplateDraft
		field string
		code  string
	}{
		{"uppercase name", domain.TemplateDraft{Name: "Welcome", Channel: domain.ChannelSMS, Body: "hi"}, "name", domain.CodeInvalidFormat},
		{"one-letter name", domain.TemplateDraft{Name: "w", Channel: domain.ChannelSMS, Body: "hi"}, "name", domain.CodeInvalidFormat},
		{"unknown channel", domain.TemplateDraft{Name: "welcome", Channel: "fax", Body: "hi"}, "channel", domain.CodeInvalidValue},
		{"empty body", domain.TemplateDraft{Name: "welcome", Channel: domain.ChannelSMS, Body: " "}, "body", domain.CodeRequired},
		{"body over channel limit", domain.TemplateDraft{
			Name: "welcome", Channel: domain.ChannelPush, Body: strings.Repeat("x", 501),
		}, "body", domain.CodeTooLong},
		{"unclosed placeholder", domain.TemplateDraft{
			Name: "welcome", Channel: domain.ChannelSMS, Body: "Hi {{name",
		}, "body", domain.CodeInvalidFormat},
		{"invalid variable name", domain.TemplateDraft{
			Name: "welcome", Channel: domain.ChannelSMS, Body: "Hi {{first name}}",
		}, "body", domain.CodeInvalidFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var verr *domain.ValidationError
			require.ErrorAs(t, tc.draft.Validate(), &verr)
			require.Len(t, verr.Fields, 1, "%v", verr.Fields)
			assert.Equal(t, tc.field, verr.Fields[0].Field)
			assert.Equal(t, tc.code, verr.Fields[0].Code)
		})
	}
}

func TestBatchSummaryProgress(t *testing.T) {
	t.Parallel()

	running := domain.BatchSummary{Counts: map[domain.Status]int{
		domain.StatusSent: 7, domain.StatusQueued: 2, domain.StatusProcessing: 1, domain.StatusFailed: 1,
	}}
	assert.Equal(t, 3, running.Pending())
	assert.False(t, running.Done())

	finished := domain.BatchSummary{Counts: map[domain.Status]int{
		domain.StatusSent: 9, domain.StatusCancelled: 1,
	}}
	assert.Zero(t, finished.Pending())
	assert.True(t, finished.Done())
}

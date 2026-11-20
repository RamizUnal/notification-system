package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

func TestTemplateService(t *testing.T) {
	t.Parallel()
	mem := newMemory()
	clk := &clock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	svc := service.NewTemplateService(mem.templateRepo(), service.WithClock(clk.Now))
	ctx := context.Background()

	created, err := svc.Create(ctx, domain.TemplateDraft{
		Name: "otp", Channel: domain.ChannelSMS, Body: "Your code is {{code}}",
	})
	require.NoError(t, err)
	assert.Equal(t, clk.Now(), created.CreatedAt)

	t.Run("rejects invalid drafts", func(t *testing.T) {
		_, err := svc.Create(ctx, domain.TemplateDraft{Name: "Bad Name", Channel: domain.ChannelSMS, Body: "x"})
		requireValidationFields(t, err, "name")
	})

	t.Run("rejects duplicate names", func(t *testing.T) {
		_, err := svc.Create(ctx, domain.TemplateDraft{Name: "otp", Channel: domain.ChannelSMS, Body: "x"})
		require.ErrorIs(t, err, domain.ErrTemplateNameTaken)
	})

	t.Run("update keeps identity and moves the timestamp", func(t *testing.T) {
		clk.Advance(time.Minute)
		updated, err := svc.Update(ctx, created.ID, domain.TemplateDraft{
			Name: "otp", Channel: domain.ChannelSMS, Body: "Code: {{code}}",
		})
		require.NoError(t, err)
		assert.Equal(t, created.ID, updated.ID)
		assert.Equal(t, created.CreatedAt, updated.CreatedAt)
		assert.Equal(t, clk.Now(), updated.UpdatedAt)
	})

	t.Run("render previews and reports missing variables", func(t *testing.T) {
		out, err := svc.Render(ctx, created.ID, map[string]string{"code": "123456"})
		require.NoError(t, err)
		assert.Equal(t, "Code: 123456", out)

		_, err = svc.Render(ctx, created.ID, nil)
		verr := requireValidationFields(t, err, "variables")
		assert.Equal(t, domain.CodeMissingVariables, verr.Fields[0].Code)
	})
}

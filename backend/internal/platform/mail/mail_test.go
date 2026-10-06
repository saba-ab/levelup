package mail

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/shared/errs"
)

func TestNewSelectsDriver(t *testing.T) {
	m, err := New(Config{}, zap.NewNop())
	require.NoError(t, err)
	require.IsType(t, Log{}, m)
	_, err = New(Config{Driver: "smtp"}, zap.NewNop())
	require.Error(t, err)
	_, err = New(Config{Driver: "carrier-pigeon"}, zap.NewNop())
	require.Error(t, err)
	m, err = New(Config{Driver: "smtp", Host: "smtp.example.com", From: "LevelUp <no-reply@example.com>"}, zap.NewNop())
	require.NoError(t, err)
	require.Equal(t, 587, m.(SMTP).cfg.Port)
}

func TestBuildMessage(t *testing.T) {
	raw := string(build("LevelUp <a@b.c>", Message{To: "x@y.z", Subject: "Hi", Text: "plain", HTML: "<b>html</b>"}))
	require.Contains(t, raw, "Subject: Hi\r\n")
	require.Contains(t, raw, "multipart/alternative")
	require.True(t, strings.Contains(raw, "plain") && strings.Contains(raw, "<b>html</b>"))
	require.Equal(t, "a@b.c", addressOf("LevelUp <a@b.c>"))
}

func TestHeaderInjectionRefused(t *testing.T) {
	err := SMTP{cfg: Config{Host: "127.0.0.1", Port: 1, From: "a@b.c"}}.Send(context.Background(), Message{To: "x@y.z\r\nBcc: evil@z", Subject: "s", Text: "t"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestFromHeader(t *testing.T) {
	require.Equal(t, "LevelUp <no-reply@levelupos.ge>", fromHeader("LevelUp <no-reply@levelupos.ge>", " "))
	require.Equal(t, `"Acme Rewards" <no-reply@levelupos.ge>`, fromHeader("LevelUp <no-reply@levelupos.ge>", "Acme Rewards"))
	require.Equal(t, "=?utf-8?q?Caf=C3=A9?= <no-reply@levelupos.ge>", fromHeader("no-reply@levelupos.ge", "Café"))
}

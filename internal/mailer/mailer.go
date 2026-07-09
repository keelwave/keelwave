package mailer

import "embed"

const (
	FromName       = "keelwave"
	maxRetries     = 3
	InviteTemplate = "invite.tmpl"
	VerifyTemplate = "verify.tmpl"
	AlertTemplate  = "alert.tmpl"
)

//go:embed "templates"
var FS embed.FS

type Client interface {
	Send(templateFile, email string, data any) error
}

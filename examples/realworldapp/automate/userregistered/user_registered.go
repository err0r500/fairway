package userregistered

import (
	"context"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/realworldapp/automate"
	"github.com/err0r500/fairway/examples/realworldapp/event"
)

func init() {
	Register(&automate.Registry)
}

// Register adds this automation to the registry (public for tests)
func Register(registry *automate.AutomationRegistry) {
	registry.Register(factory)
}

// Deps for this automation
type Deps struct {
	EmailSender automate.EmailSender
}

func factory(store dcb.DcbStore, allDeps automate.AllDeps) (automate.Startable, error) {
	return fairway.NewAutomation(
		store,
		Deps{EmailSender: allDeps.EmailSender},
		"welcome-email",
		event.UserRegistered{},
		eventToCommand,
	)
}

func eventToCommand(ev fairway.Event) fairway.CommandWithEffect[Deps] {
	data := ev.Data.(event.UserRegistered)
	return sendWelcomeEmailCmd{UserId: data.Id, Email: data.Email, Name: data.Name}
}

type sendWelcomeEmailCmd struct {
	UserId, Email, Name string
}

func (c sendWelcomeEmailCmd) Run(ctx context.Context, ra fairway.EventReadAppender, deps Deps) error {
	alreadySent := false

	if err := ra.ReadEvents(ctx, fairway.QueryItems(
		fairway.NewQueryItem().Types(event.UserWelcomeEmailSent{}).Tags(event.UserIdTag(c.UserId)),
	), func(e fairway.Event) bool {
		// if a single event is returned, it means the email has already been sent
		alreadySent = true
		return false
	}); err != nil {
		return err
	}

	if alreadySent {
		return nil
	}

	return deps.EmailSender.SendWelcomeEmail(ctx, c.Email, c.Name)
}

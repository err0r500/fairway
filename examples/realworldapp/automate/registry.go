package automate

import (
	"context"
	"fmt"
	"log"

	"github.com/err0r500/fairway/dcb"
)

// Startable interface for automations
type Startable interface {
	QueueId() string
	Start(ctx context.Context) error
	Stop()
	Wait() error
}

// AllDeps contains all services needed by automations
type AllDeps struct {
	EmailSender EmailSender
}

// EmailSender interface
type EmailSender interface {
	SendWelcomeEmail(ctx context.Context, email, name string) error
}

// AutomationFactory creates an automation
type AutomationFactory func(store dcb.DcbStore, deps AllDeps) (Startable, error)

// AutomationRegistry holds registered automation factories
type AutomationRegistry struct {
	factories []AutomationFactory
}

func (r *AutomationRegistry) RegisterAutomation(f AutomationFactory) {
	r.factories = append(r.factories, f)
}

// StartAll creates and starts all automations, returns stop func
func (r *AutomationRegistry) StartAll(ctx context.Context, store dcb.DcbStore, deps AllDeps) (func(), error) {
	log.Println("starting all automations")

	var automations []Startable
	seen := make(map[string]bool)
	for _, f := range r.factories {
		a, err := f(store, deps)
		if err != nil {
			return nil, err
		}
		qid := a.QueueId()
		if seen[qid] {
			return nil, fmt.Errorf("duplicate automation queueId: %q", qid)
		}
		seen[qid] = true
		if err := a.Start(ctx); err != nil {
			return nil, err
		}
		automations = append(automations, a)
	}
	return func() {
		for _, a := range automations {
			a.Stop()
		}
		for _, a := range automations {
			a.Wait()
		}
	}, nil
}

var Registry = AutomationRegistry{}

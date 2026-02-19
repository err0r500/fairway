package automate

import (
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/shoppingcart/view/opencartswithproducts"
)

// AllDeps contains all services needed by automations
type AllDeps struct {
	OpenCartsRM *fairway.ReadModel[opencartswithproducts.CartItem, opencartswithproducts.Repo]
}

var Registry = fairway.AutomationRegistry[AllDeps]{}

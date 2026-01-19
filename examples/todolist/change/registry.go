package change

import "github.com/err0r500/fairway"

// commandRegistry stores all registered command routes
// the modules register on init()
// the main function use register them as httpHandler on execution
var ChangeRegistry = fairway.HttpChangeRegistry{}

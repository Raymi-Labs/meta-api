package common

// Only verified concrete decision models belong here. Jev Router is a chat
// router with variable pricing, and must never inherit this endpoint.
func IsJevDecisionModel(name string) bool {
	switch name {
	case "typesafe/jev-1.13", "typesafe/jev-1.14":
		return true
	default:
		return false
	}
}

package backend

import "fmt"

type BackendCapabilities struct {
	CanCreate             bool
	CanUpdate             bool
	CanDelete             bool
	CanClose              bool
	CanSearch             bool
	CanQuery              bool
	CanListReady          bool
	CanManageDependencies bool
	CanManageLabels       bool
	CanSync               bool
	MaxConcurrency        int
}

var FullCapabilities = BackendCapabilities{
	CanCreate:             true,
	CanUpdate:             true,
	CanDelete:             true,
	CanClose:              true,
	CanSearch:             true,
	CanQuery:              true,
	CanListReady:          true,
	CanManageDependencies: true,
	CanManageLabels:       true,
	CanSync:               true,
	MaxConcurrency:        0,
}

var ReadOnlyCapabilities = BackendCapabilities{
	CanCreate:             false,
	CanUpdate:             false,
	CanDelete:             false,
	CanClose:              false,
	CanSearch:             true,
	CanQuery:              true,
	CanListReady:          true,
	CanManageDependencies: false,
	CanManageLabels:       false,
	CanSync:               false,
	MaxConcurrency:        0,
}

func HasCapability(cap BackendCapabilities, flag string) bool {
	switch flag {
	case "CanCreate":
		return cap.CanCreate
	case "CanUpdate":
		return cap.CanUpdate
	case "CanDelete":
		return cap.CanDelete
	case "CanClose":
		return cap.CanClose
	case "CanSearch":
		return cap.CanSearch
	case "CanQuery":
		return cap.CanQuery
	case "CanListReady":
		return cap.CanListReady
	case "CanManageDependencies":
		return cap.CanManageDependencies
	case "CanManageLabels":
		return cap.CanManageLabels
	case "CanSync":
		return cap.CanSync
	case "MaxConcurrency":
		return cap.MaxConcurrency > 0
	default:
		return false
	}
}

func AssertCapability(cap BackendCapabilities, flag string, operation string) error {
	if !HasCapability(cap, flag) {
		return fmt.Errorf("Backend does not support %s (missing capability: %s)", operation, flag)
	}
	return nil
}

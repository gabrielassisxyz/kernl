package backend

import "testing"

func TestAssertCapability_ThrowsWhenBooleanFlagFalse(t *testing.T) {
	err := AssertCapability(ReadOnlyCapabilities, "CanCreate", "create bead")
	if err == nil {
		t.Fatal("expected error for false boolean capability, got nil")
	}
	expected := "Backend does not support create bead (missing capability: CanCreate)"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestAssertCapability_ThrowsWhenMaxConcurrencyZero(t *testing.T) {
	err := AssertCapability(FullCapabilities, "MaxConcurrency", "concurrent operations")
	if err == nil {
		t.Fatal("expected error for maxConcurrency 0, got nil")
	}
	expected := "Backend does not support concurrent operations (missing capability: MaxConcurrency)"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestAssertCapability_DoesNotThrowWhenBooleanFlagTrue(t *testing.T) {
	err := AssertCapability(FullCapabilities, "CanCreate", "create bead")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAssertCapability_DoesNotThrowWhenMaxConcurrencyPositive(t *testing.T) {
	caps := BackendCapabilities{
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
		MaxConcurrency:        4,
	}
	err := AssertCapability(caps, "MaxConcurrency", "concurrent operations")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAssertCapability_IncludesFlagAndOperationInMessage(t *testing.T) {
	err := AssertCapability(ReadOnlyCapabilities, "CanDelete", "delete bead")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !containsStr(msg, "delete bead") {
		t.Errorf("expected operation name in error, got: %s", msg)
	}
	if !containsStr(msg, "CanDelete") {
		t.Errorf("expected capability flag in error, got: %s", msg)
	}
}

func TestHasCapability_ReturnsTrueForEnabledBooleanFlags(t *testing.T) {
	if !HasCapability(FullCapabilities, "CanCreate") {
		t.Error("expected CanCreate to be true")
	}
	if !HasCapability(FullCapabilities, "CanSearch") {
		t.Error("expected CanSearch to be true")
	}
}

func TestHasCapability_ReturnsFalseForDisabledBooleanFlags(t *testing.T) {
	if HasCapability(ReadOnlyCapabilities, "CanCreate") {
		t.Error("expected CanCreate to be false for read-only")
	}
	if HasCapability(ReadOnlyCapabilities, "CanSync") {
		t.Error("expected CanSync to be false for read-only")
	}
}

func TestHasCapability_ReturnsFalseWhenMaxConcurrencyZero(t *testing.T) {
	if HasCapability(FullCapabilities, "MaxConcurrency") {
		t.Error("expected MaxConcurrency=0 to return false")
	}
}

func TestHasCapability_ReturnsTrueWhenMaxConcurrencyPositive(t *testing.T) {
	caps := BackendCapabilities{
		MaxConcurrency: 8,
	}
	if !HasCapability(caps, "MaxConcurrency") {
		t.Error("expected MaxConcurrency=8 to return true")
	}
}

func TestHasCapability_ReturnsFalseForUnknownFlag(t *testing.T) {
	if HasCapability(FullCapabilities, "NonExistent") {
		t.Error("expected unknown flag to return false")
	}
}

func TestFullCapabilities_AllBooleansTrue(t *testing.T) {
	flags := []string{
		"CanCreate", "CanUpdate", "CanDelete", "CanClose",
		"CanSearch", "CanQuery", "CanListReady",
		"CanManageDependencies", "CanManageLabels", "CanSync",
	}
	for _, flag := range flags {
		if !HasCapability(FullCapabilities, flag) {
			t.Errorf("expected %s to be true in FullCapabilities", flag)
		}
	}
}

func TestFullCapabilities_MaxConcurrencyZero(t *testing.T) {
	if FullCapabilities.MaxConcurrency != 0 {
		t.Errorf("expected MaxConcurrency 0, got %d", FullCapabilities.MaxConcurrency)
	}
}

func TestReadOnlyCapabilities_WriteFlagsFalse(t *testing.T) {
	writeFlags := []string{
		"CanCreate", "CanUpdate", "CanDelete", "CanClose",
		"CanManageDependencies", "CanManageLabels", "CanSync",
	}
	for _, flag := range writeFlags {
		if HasCapability(ReadOnlyCapabilities, flag) {
			t.Errorf("expected %s to be false in ReadOnlyCapabilities", flag)
		}
	}
}

func TestReadOnlyCapabilities_ReadFlagsTrue(t *testing.T) {
	readFlags := []string{"CanSearch", "CanQuery", "CanListReady"}
	for _, flag := range readFlags {
		if !HasCapability(ReadOnlyCapabilities, flag) {
			t.Errorf("expected %s to be true in ReadOnlyCapabilities", flag)
		}
	}
}

func TestReadOnlyCapabilities_MaxConcurrencyZero(t *testing.T) {
	if ReadOnlyCapabilities.MaxConcurrency != 0 {
		t.Errorf("expected MaxConcurrency 0, got %d", ReadOnlyCapabilities.MaxConcurrency)
	}
}

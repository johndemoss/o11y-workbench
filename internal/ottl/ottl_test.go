package ottl

import (
	"testing"
)

func TestValidateStatements_Valid(t *testing.T) {
	result := ValidateStatements([]string{
		`set(span.attributes["env"], "prod")`,
	})
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
	if result.Parsed != 1 {
		t.Errorf("expected 1 parsed, got %d", result.Parsed)
	}
}

func TestValidateStatements_Invalid_Syntax(t *testing.T) {
	result := ValidateStatements([]string{
		`bad syntax!!!`,
	})
	if result.Valid {
		t.Errorf("expected invalid for bad syntax")
	}
	if len(result.Errors) == 0 {
		t.Errorf("expected error messages for bad syntax")
	}
}

func TestValidateStatements_Invalid_UndefinedFunction(t *testing.T) {
	result := ValidateStatements([]string{
		`noSuchFunction(span.attributes["x"])`,
	})
	if result.Valid {
		t.Errorf("expected invalid for undefined function")
	}
	if len(result.Errors) == 0 {
		t.Errorf("expected error messages for undefined function")
	}
}

func TestValidateStatements_Invalid_WrongArgs(t *testing.T) {
	result := ValidateStatements([]string{
		`set(span.attributes["x"])`, // set requires 2 args (target, value)
	})
	if result.Valid {
		t.Errorf("expected invalid for wrong argument count")
	}
	if len(result.Errors) == 0 {
		t.Errorf("expected error messages for wrong argument count")
	}
}

func TestValidateStatements_Multiple_OneInvalid(t *testing.T) {
	result := ValidateStatements([]string{
		`set(span.attributes["a"], "b")`,
		`bad syntax!!!`,
	})
	if result.Valid {
		t.Errorf("expected invalid when any statement fails")
	}
}

func TestValidateStatements_IsMatch(t *testing.T) {
	result := ValidateStatements([]string{
		`set(span.attributes["matched"], "yes") where IsMatch(span.name, "foo.*")`,
	})
	if !result.Valid {
		t.Errorf("expected valid IsMatch statement, got errors: %v", result.Errors)
	}
}

func TestValidateStatements_DeleteKey(t *testing.T) {
	result := ValidateStatements([]string{
		`delete_key(span.attributes, "secret")`,
	})
	if !result.Valid {
		t.Errorf("expected valid delete_key statement, got errors: %v", result.Errors)
	}
}

func TestValidateStatements_Empty(t *testing.T) {
	result := ValidateStatements(nil)
	if result.Valid {
		t.Errorf("expected invalid for nil statements")
	}
	result = ValidateStatements([]string{})
	if result.Valid {
		t.Errorf("expected invalid for empty statements")
	}
}

func TestValidateStatements_Converters(t *testing.T) {
	result := ValidateStatements([]string{
		`set(span.attributes["len"], Len(span.attributes["items"]))`,
	})
	if !result.Valid {
		t.Errorf("expected valid with converter function, got errors: %v", result.Errors)
	}
}

func TestValidateStatements_WhereClause(t *testing.T) {
	result := ValidateStatements([]string{
		`set(span.attributes["x"], "y") where span.attributes["x"] == nil`,
	})
	if !result.Valid {
		t.Errorf("expected valid with where clause, got errors: %v", result.Errors)
	}
}

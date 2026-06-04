package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCalculator_BasicOps(t *testing.T) {
	c := NewCalculator()
	cases := []struct {
		expr string
		want string
	}{
		{"1+2", "3"},
		{"1+2*3", "7"},
		{"(1+2)*3", "9"},
		{"10/4", "2.5"},
		{"-5+3", "-2"},
		{"2.5*4", "10"},
	}
	for _, tc := range cases {
		args, _ := json.Marshal(map[string]string{"expr": tc.expr})
		got, err := c.Execute(context.Background(), args)
		if err != nil {
			t.Errorf("%s: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestCalculator_DivByZero(t *testing.T) {
	c := NewCalculator()
	args, _ := json.Marshal(map[string]string{"expr": "1/0"})
	_, err := c.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("err = %v, want division by zero", err)
	}
}

func TestCalculator_InvalidExpr(t *testing.T) {
	c := NewCalculator()
	args, _ := json.Marshal(map[string]string{"expr": "1++"})
	_, err := c.Execute(context.Background(), args)
	if err == nil {
		t.Error("want err for invalid expr")
	}
}

func TestCalculator_BadArgs(t *testing.T) {
	c := NewCalculator()
	_, err := c.Execute(context.Background(), json.RawMessage(`{"bad":1}`))
	if err == nil {
		t.Error("want err when expr missing")
	}
}

func TestCalculator_SchemaIsValidJSON(t *testing.T) {
	c := NewCalculator()
	var m map[string]any
	if err := json.Unmarshal(c.Schema(), &m); err != nil {
		t.Errorf("Schema not valid JSON: %v", err)
	}
}

package perf

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestCheckTokenBudget(t *testing.T) {
	ctx := WithUsage(context.Background(), &Usage{})
	check := func(want bool, why string) {
		t.Helper()
		if err := CheckTokenBudget(ctx); errors.Is(err, ErrTokenBudgetExceeded) != want {
			t.Fatalf("%s: err = %v, want refused=%v", why, err, want)
		}
	}

	t.Setenv(EnvMaxTotalTokens, "")
	AddUsage(ctx, "m", 900, 900)
	check(false, "no budget set")

	t.Setenv(EnvMaxTotalTokens, "2000")
	check(false, "1800 of 2000 used")
	AddUsage(ctx, "m", 100, 100)
	check(true, "2000 of 2000 used")

	t.Setenv(EnvMaxTotalTokens, "abc")
	check(true, "invalid budget fails closed")

	t.Setenv(EnvMaxTotalTokens, "1000000")
	unreported := WithUsage(context.Background(), &Usage{})
	AddUsageWithReport(unreported, "m", 0, 0, false)
	if err := CheckTokenBudget(unreported); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("unreported usage: err = %v, want refusal", err)
	}
	negative := WithUsage(context.Background(), &Usage{})
	AddUsage(negative, "m", -5000, 10)
	if err := CheckTokenBudget(negative); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("negative usage report: err = %v, want refusal", err)
	}
	overflow := WithUsage(context.Background(), &Usage{})
	AddUsage(overflow, "m", math.MaxInt64/2+1, 0)
	AddUsage(overflow, "m", math.MaxInt64/2+1, 0)
	if err := CheckTokenBudget(overflow); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("overflowed usage total: err = %v, want refusal", err)
	}
	if err := CheckTokenBudget(context.Background()); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("no usage accounting: err = %v, want refusal", err)
	}
}

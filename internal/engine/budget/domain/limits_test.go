package domain

import (
	"testing"
	"time"
)

func TestProductionBudgetLimits(t *testing.T) {
	if CallDuration != 10*time.Second || StartupDuration != 120*time.Second || GraceDuration != 10*time.Second {
		t.Fatal("production controller budgets changed", CallDuration, StartupDuration, GraceDuration)
	}
}

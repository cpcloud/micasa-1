// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDateDiffDays(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 14, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		target time.Time
		want   int
	}{
		{"same day", now, 0},
		{"tomorrow", now.AddDate(0, 0, 1), 1},
		{"yesterday", now.AddDate(0, 0, -1), -1},
		{"30 days ahead", now.AddDate(0, 0, 30), 30},
		{"30 days behind", now.AddDate(0, 0, -30), -30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DateDiffDays(now, tt.target))
		})
	}
}

func TestDaysText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		days int
		want string
	}{
		{"zero", 0, "today"},
		{"positive 1d", 1, "1d"},
		{"negative 1d", -1, "1d"},
		{"positive 15d", 15, "15d"},
		{"negative 15d", -15, "15d"},
		{"months", 45, "1mo"},
		{"year", 400, "1y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DaysText(tt.days))
		})
	}
}

func TestShortDur(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "now"},
		{"sub-minute", 30 * time.Second, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48 * time.Hour, "2d"},
		{"months", 60 * 24 * time.Hour, "2mo"},
		{"year", 400 * 24 * time.Hour, "1y"},
		{"negative", -5 * time.Minute, "5m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ShortDur(tt.d))
		})
	}
}

func TestPastDur(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "<1m", PastDur(10*time.Second))
	assert.Equal(t, "5m", PastDur(5*time.Minute))
}

package main

import (
	"strings"
	"testing"
)

func TestAboutMessage_IncludesVersionAttributionAndSupport(t *testing.T) {
	got := aboutMessage("0.1.12", "tech@sunrise.co.za")
	for _, want := range []string{
		"Version 0.1.12",
		"Network readiness diagnostic",
		"Tim Keller",
		"Sunrise Animation Studios",
		"tech@sunrise.co.za",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("aboutMessage missing %q in:\n%s", want, got)
		}
	}
}

func TestAboutMessage_OmitsSupportLineWhenEmailEmpty(t *testing.T) {
	got := aboutMessage("0.1.12", "")
	if strings.Contains(got, "Support") {
		t.Errorf("aboutMessage should omit support line when email empty:\n%s", got)
	}
	if !strings.Contains(got, "Version 0.1.12") {
		t.Errorf("aboutMessage missing version:\n%s", got)
	}
}

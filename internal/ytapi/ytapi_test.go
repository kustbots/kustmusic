package ytapi

import "testing"

func TestIsoDuration(t *testing.T) {
	cases := map[float64]string{
		0:    "",
		-5:   "",
		45:   "PT45S",
		60:   "PT1M0S",
		213:  "PT3M33S",
		3600: "PT1H0M0S",
		3725: "PT1H2M5S",
	}
	for in, want := range cases {
		if got := isoDuration(in); got != want {
			t.Errorf("isoDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

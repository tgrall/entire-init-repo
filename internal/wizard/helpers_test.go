package wizard

import "testing"

func TestRefFromEntireURL(t *testing.T) {
	cases := map[string]string{
		"entire://aws-eu-central-1.entire.io/et/p/r":     "/et/p/r",
		"entire://aws-eu-central-1.entire.io/gh/o/r.git": "/gh/o/r",
		"entire://host": "",
	}
	for in, want := range cases {
		if got := refFromEntireURL(in); got != want {
			t.Errorf("refFromEntireURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"abc", "my-app", "a1-b2"} {
		if err := validateName(ok); err != nil {
			t.Errorf("validateName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"ab", "-abc", "abc-", "My-App", "a_b"} {
		if validateName(bad) == nil {
			t.Errorf("validateName(%q) should fail", bad)
		}
	}
}

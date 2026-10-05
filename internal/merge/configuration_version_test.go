package merge

import (
	"strings"
	"testing"

	"xmlmerge/internal/rules"
)

func TestConfigurationVersionProperty(t *testing.T) {
	db, err := rules.Load("../../rules.xml")
	if err != nil {
		t.Fatal(err)
	}
	document := func(version, note string) []byte {
		return []byte(`<MetaDataObject xmlns="http://v8.1c.ru/8.3/MDClasses" version="2.20"><Configuration><Properties><Name>ERP</Name><Version>` + version + `</Version><Comment>` + note + `</Comment></Properties></Configuration></MetaDataObject>`)
	}
	for _, tc := range []struct {
		name, local, remote string
		conflict            bool
	}{
		{"supplier release update", "2.5.22.166", "2.5.22.176", false},
		{"same release update", "2.5.22.176", "2.5.22.176", false},
		{"different release updates", "2.5.22.186", "2.5.22.176", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Merge(document("2.5.22.166", "base"), document(tc.local, "base"), document(tc.remote, "base"), db)
			if err != nil {
				t.Fatal(err)
			}
			if (len(result.Conflicts) > 0) != tc.conflict {
				t.Fatalf("conflicts: %+v", result.Conflicts)
			}
			if !tc.conflict && !strings.Contains(string(result.Data), "<Version>"+tc.remote+"</Version>") {
				t.Fatalf("unexpected result: %s", result.Data)
			}
			for _, conflict := range result.Conflicts {
				if strings.Contains(conflict.Reason, "identity change") {
					t.Fatalf("property value treated as identity: %+v", conflict)
				}
			}
		})
	}
	result, err := Merge(document("2.5.22.166", "custom"), document("2.5.22.166", "custom"), document("2.5.22.176", "custom"), db)
	if err != nil || len(result.Conflicts) != 0 || !strings.Contains(string(result.Data), "<Comment>custom</Comment>") {
		t.Fatalf("custom property lost: result=%+v err=%v", result, err)
	}
	duplicate := strings.Replace(string(document("2.5.22.166", "base")), "</Version>", "</Version><Version>other</Version>", 1)
	result, err = Merge([]byte(duplicate), []byte(duplicate), document("2.5.22.176", "base"), db)
	if err != nil || len(result.Conflicts) == 0 {
		t.Fatalf("ambiguous duplicate property accepted: result=%+v err=%v", result, err)
	}
}

func TestConfigurationMobileFunctionalities(t *testing.T) {
	db, err := rules.Load("../../rules.xml")
	if err != nil {
		t.Fatal(err)
	}
	document := func(biometrics, location string) []byte {
		return []byte(`<MetaDataObject xmlns="http://v8.1c.ru/8.3/MDClasses" xmlns:app="http://v8.1c.ru/8.2/managed-application/core" version="2.20"><Configuration><Properties><Name>ERP</Name><UsedMobileApplicationFunctionalities><app:functionality><app:functionality>Biometrics</app:functionality><app:use>` + biometrics + `</app:use></app:functionality><app:functionality><app:functionality>Location</app:functionality><app:use>` + location + `</app:use></app:functionality></UsedMobileApplicationFunctionalities></Properties></Configuration></MetaDataObject>`)
	}
	base := document("true", "false")
	result, err := Merge(base, document("false", "false"), document("true", "true"), db)
	if err != nil || len(result.Conflicts) != 0 {
		t.Fatalf("independent functionality changes: result=%+v err=%v", result, err)
	}
	if string(result.Data) != string(document("false", "true")) {
		t.Fatalf("unexpected merged flags: %s", result.Data)
	}
	result, err = Merge(base, base, base, db)
	if err != nil || len(result.Conflicts) != 0 || string(result.Data) != string(base) {
		t.Fatalf("unchanged functionalities conflict: result=%+v err=%v", result, err)
	}
}

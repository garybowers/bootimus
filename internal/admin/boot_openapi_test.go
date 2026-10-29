package admin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBootAuthenticationOpenAPI(test *testing.T) {
	document := BuildOpenAPI("test", "")
	operation, exists := document.Paths["/boot-auth"]["post"]
	if !exists || !operation.Public {
		test.Fatal("boot authentication must be documented independently of admin authentication")
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		test.Fatal(err)
	}
	for _, expected := range []string{"application/x-www-form-urlencoded", "text/plain"} {
		if !strings.Contains(string(encoded), expected) {
			test.Errorf("missing content type %s", expected)
		}
	}
}

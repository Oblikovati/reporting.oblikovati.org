// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestPayloadRoundTrip guards the JSON contract with the application: the fields the app
// sends must survive a marshal/unmarshal here, with the PNGs base64-encoded as []byte.
func TestPayloadRoundTrip(t *testing.T) {
	in := Payload{
		Comment:        "broke on extrude",
		OS:             "linux",
		Arch:           "amd64",
		AppVersion:     "0.16.0",
		OpenDocuments:  []DocumentInfo{{Path: "/tmp/p.obk", Name: "p", Type: "Part", Dirty: true}},
		TransactionLog: []string{"Sketch", "Extrude"},
		WindowPNG:      []byte{0x89, 0x50, 0x4e, 0x47},
		ViewportPNG:    []byte{0x01, 0x02},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// json tag contract: lowerCamel keys the app also uses.
	if !strings.Contains(string(raw), `"windowPng"`) || !strings.Contains(string(raw), `"openDocuments"`) {
		t.Fatalf("unexpected JSON keys: %s", raw)
	}
	var out Payload
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Comment != in.Comment || !bytes.Equal(out.WindowPNG, in.WindowPNG) {
		t.Errorf("round trip mismatch: %+v", out)
	}
	if len(out.OpenDocuments) != 1 || !out.OpenDocuments[0].Dirty {
		t.Errorf("document info lost: %+v", out.OpenDocuments)
	}
}

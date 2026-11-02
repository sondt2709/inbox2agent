package phone

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "write testdata/vectors.json again")

func testKey() []byte {
	k := make([]byte, KeySize)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestSealRoundTrip(t *testing.T) {
	key := testKey()
	plain := []byte(`{"v":1,"kind":"test","ts":1}`)
	s, err := Seal(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(key, s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("Open = %q, want %q", got, plain)
	}
	s2, _ := Seal(key, plain)
	if s == s2 {
		t.Fatal("two seals of one text are equal; the nonce must be random")
	}
}

func TestOpenWrongKey(t *testing.T) {
	s, err := Seal(testKey(), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	other := testKey()
	other[0] ^= 1
	if _, err := Open(other, s); err == nil {
		t.Fatal("Open with another key passed")
	}
	raw, _ := base64.StdEncoding.DecodeString(s)
	raw[len(raw)-1] ^= 1
	if _, err := Open(testKey(), base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("Open of a changed message passed")
	}
}

func TestSealBadKey(t *testing.T) {
	if _, err := Seal(make([]byte, 16), []byte("x")); err == nil {
		t.Fatal("Seal with a 16-byte key passed")
	}
}

// noise makes text that deflate cannot compress much.
func noise(n int) string {
	var b strings.Builder
	x := uint32(7)
	for b.Len() < n {
		x = x*1103515245 + 12345
		b.WriteByte("abcdefghijklmnopqrstuvwxyz ABCDEFGHIJ0123456789"[x>>16%47])
	}
	return b.String()
}

func TestSealFitCutsLargeCard(t *testing.T) {
	c := &Card{ID: 1, Cat: "action", Urg: "high", Title: "Large", Body: noise(4000), Reply: "ok", Created: 1, State: "new"}
	for i := 0; i < 10; i++ {
		c.Refs = append(c.Refs, Ref{Author: "a", Text: noise(600), URL: "https://example.com/p/1", At: 1})
	}
	p := &Payload{V: Version, Kind: KindCard, TS: 1, Card: c}
	s, err := SealFit(testKey(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) > MaxSealed {
		t.Fatalf("sealed length %d > %d", len(s), MaxSealed)
	}
	if !p.Card.Cut {
		t.Fatal("Cut is false")
	}
	if len(p.Card.Refs) > MaxRefs {
		t.Fatalf("%d refs, want at most %d", len(p.Card.Refs), MaxRefs)
	}
	var got Payload
	plain, err := Open(testKey(), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(plain, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Card.Cut || got.Card.Title != "Large" {
		t.Fatalf("opened card = %+v", got.Card)
	}
}

func TestSealFitKeepsSmallCard(t *testing.T) {
	p := &Payload{V: Version, Kind: KindCard, TS: 1, Card: &Card{ID: 1, Cat: "info", Urg: "low", Title: "Small", Body: "short", Created: 1, State: "new"}}
	if _, err := SealFit(testKey(), p); err != nil {
		t.Fatal(err)
	}
	if p.Card.Cut || p.Card.Body != "short" {
		t.Fatalf("small card changed: %+v", p.Card)
	}
}

// vectorFile is the format of testdata/vectors.json. The Kotlin tests in
// mobile/shared read the same file, so both sides must open each vector.
type vectorFile struct {
	Comment string   `json:"$comment"`
	Key     string   `json:"key"`
	AAD     string   `json:"aad"`
	Vectors []vector `json:"vectors"`
}

type vector struct {
	Name    string          `json:"name"`
	C       string          `json:"c"`
	Payload json.RawMessage `json:"payload"`
}

// counterReader gives the bytes 0, 1, 2, ... so that the vectors do not change.
type counterReader struct{ n byte }

func (r *counterReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.n
		r.n++
	}
	return len(p), nil
}

func vectorPayloads() []Payload {
	return []Payload{
		{V: 1, Kind: KindTest, TS: 1793700000, Alert: true},
		{V: 1, Kind: KindCard, TS: 1793700001, Alert: true, Card: &Card{
			ID: 812, Key: "mm:release-window", Cat: "action", Urg: "high", Title: "Approve the release window for Friday",
			Group: "Harbor Portal", Body: "Lena asks if the **portal release** can go on Friday 18:00.\n\n- QA passed\n- The rollback plan is ready",
			Reply: "Yes, Friday 18:00 is OK.", Created: 1793699000, State: "new",
			Refs: []Ref{{Author: "lena", Text: "Can we release on Friday 18:00?", URL: "https://chat.example.com/team/pl/abc", Channel: "town-square", At: 1793698000}},
		}},
		{V: 1, Kind: KindCard, TS: 1793700002, Alert: false, Card: &Card{
			ID: 813, Cat: "info", Urg: "low", Title: "Bữa tối dời sang 19:30 ✓", Group: "Gia đình", Body: "Mẹ dời bữa tối sang 19:30.", Created: 1793699100, State: "new",
		}},
	}
}

func TestVectors(t *testing.T) {
	path := filepath.Join("testdata", "vectors.json")
	if *update {
		old := randReader
		randReader = &counterReader{}
		defer func() { randReader = old }()
		f := vectorFile{
			Comment: "Sealed messages for the Go and Kotlin tests. Write again with: go test ./internal/phone -run TestVectors -update",
			Key:     base64.StdEncoding.EncodeToString(testKey()), AAD: AAD,
		}
		for _, p := range vectorPayloads() {
			b, _ := json.Marshal(p)
			c, err := Seal(testKey(), b)
			if err != nil {
				t.Fatal(err)
			}
			f.Vectors = append(f.Vectors, vector{Name: p.Kind + "-" + map[bool]string{true: "alert", false: "silent"}[p.Alert], C: c, Payload: b})
		}
		out, _ := json.MarshalIndent(f, "", "  ")
		if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f vectorFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	key, _ := base64.StdEncoding.DecodeString(f.Key)
	if len(f.Vectors) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range f.Vectors {
		plain, err := Open(key, v.C)
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		var got, want Payload
		if err := json.Unmarshal(plain, &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(v.Payload, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: opened %+v, want %+v", v.Name, got, want)
		}
	}
}

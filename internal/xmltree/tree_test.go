package xmltree

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

func TestRoundTrip(t *testing.T) {
	samples := []string{
		`<root/>`,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<root z = 'one' a=\"two\">\r\n <v><![CDATA[строка\r\nещё\nконец\r]]></v>\n<!-- comment -->\r<?test value?>\n</root>\r\n",
		`<r xmlns="urn:one" xmlns:p="urn:two"><p:c p:a="&quot; &amp; &#13;"/><c> a &lt; b </c></r>`,
		"<r>до <b>важно</b> после\r\n</r>",
		"<r a='1'\r\n b = \"2\" />",
	}
	for _, source := range samples {
		t.Run(source[:min(20, len(source))], func(t *testing.T) {
			d, err := Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			got, err := d.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != source {
				t.Fatalf("roundtrip differs:\n%q\n%q", source, got)
			}
		})
	}
}

func TestEncodings(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    Format
	}{
		{"UTF-8", Format{Encoding: "UTF-8", BOM: []byte{0xef, 0xbb, 0xbf}}},
		{"windows-1251", Format{Encoding: "windows-1251", codec: charmap.Windows1251}},
		{"UTF-16", Format{Encoding: "UTF-16LE", BOM: []byte{0xff, 0xfe}, codec: unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)}},
		{"UTF-16", Format{Encoding: "UTF-16BE", BOM: []byte{0xfe, 0xff}, codec: unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)}},
		{"UTF-16LE", Format{Encoding: "UTF-16LE", codec: unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)}},
	} {
		t.Run(tc.f.Encoding, func(t *testing.T) {
			source := "<?xml version='1.0' encoding='" + tc.name + "'?>\r\n<r><![CDATA[Привет\nмир\r\nещё\r]]></r>"
			data, err := tc.f.Encode(source)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			got, err := doc.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, got) {
				t.Fatal("encoding or newline changed")
			}
		})
	}
}

func TestRejectInvalidXML(t *testing.T) {
	for _, source := range []string{`<a/><b/>`, `text<a/>`, `<a>`, `<a></b>`, `<p:a/>`, `<a x="1" x="2"/>`, `<a xmlns:p="u" xmlns:q="u" p:x="1" q:x="2"/>`, `<!DOCTYPE r><r/>`, `<?xml version="1.0" encoding="unknown"?><r/>`} {
		if _, err := Parse([]byte(source)); err == nil {
			t.Errorf("accepted %s", source)
		}
	}
	if _, err := Parse([]byte{'<', 'r', '>', 0xff, '<', '/', 'r', '>'}); err == nil {
		t.Fatal("invalid UTF8 accepted")
	}
}

func TestUnrepresentableCharacter(t *testing.T) {
	f := Format{Encoding: "windows-1251", codec: charmap.Windows1251}
	if _, err := f.Encode("<r>😀</r>"); err == nil {
		t.Fatal("expected an encoding error")
	}
}

func TestDirectTextAndNames(t *testing.T) {
	d, err := Parse([]byte(`<r xmlns="urn:r"><item>before<code>ABC</code>after</item></r>`))
	if err != nil {
		t.Fatal(err)
	}
	n := d.Root.Children()[0]
	if n.Name.URI != "urn:r" || n.DirectText() != "beforeafter" {
		t.Fatalf("unexpected DOM: %+v", n)
	}
}

func TestAttributeEditPreservesLayout(t *testing.T) {
	d, err := Parse([]byte("<r z = '1'\r\n a=\"2\"><![CDATA[a\r\nb\nc\r]]></r>"))
	if err != nil {
		t.Fatal(err)
	}
	d.Root.ReplaceAttribute(Name{Local: "z"}, &Attribute{Value: "new & ' value"})
	got, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	want := "<r z = 'new &amp; &apos; value'\r\n a=\"2\"><![CDATA[a\r\nb\nc\r]]></r>"
	if string(got) != want {
		t.Fatalf("%q", got)
	}
	if _, err := Parse(got); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkParseWide(b *testing.B) {
	data := []byte("<root>" + strings.Repeat("<item id=\"42\"><code>значение</code><![CDATA[a\r\nb\nc]]></item>\r\n", 10000) + "</root>")
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		if _, err := Parse(data); err != nil {
			b.Fatal(err)
		}
	}
}

func TestNamespaceScopeDoesNotLeakToSiblings(t *testing.T) {
	doc, err := Parse([]byte(`<r xmlns="outer"><a xmlns="inner"/><b/></r>`))
	if err != nil {
		t.Fatal(err)
	}
	children := doc.Root.Children()
	if children[0].Name.URI != "inner" || children[1].Name.URI != "outer" {
		t.Fatal("namespace scope leaked")
	}
	for _, source := range []string{`<r xmlns:xml="bad"/>`, `<r xmlns:xmlns="bad"/>`, `<r xmlns:p=""/>`, `<xmlns:r/>`} {
		if _, err := Parse([]byte(source)); err == nil {
			t.Fatalf("accepted invalid namespace: %s", source)
		}
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, seed := range []string{`<r/>`, "<r><![CDATA[текст\r\nещё\n]]></r>", `<r xmlns="urn:one" a='&amp;'><child/></r>`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			return
		}
		got, err := doc.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Fatal("accepted input did not round-trip exactly")
		}
	})
}

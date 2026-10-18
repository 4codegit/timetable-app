package pdf

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/signintech/gopdf"
)

// TestGopdfAddPageIsolation verifies that repeated AddPage() calls really
// produce multiple pages in the written PDF (sanity check of the library).
func TestGopdfAddPageIsolation(t *testing.T) {
	p := gopdf.GoPdf{}
	p.Start(gopdf.Config{Unit: gopdf.UnitMM, PageSize: gopdf.Rect{W: 297, H: 420}})
	if err := p.AddTTFFontDataWithOption("DejaVu", fontRegular, gopdf.TtfOption{Style: gopdf.Regular}); err != nil {
		t.Fatal(err)
	}
	p.AddPage()
	_ = p.SetFont("DejaVu", "", 10)
	p.SetX(10)
	p.SetY(10)
	_ = p.Text("страница 1")
	p.AddPage()
	p.SetX(10)
	p.SetY(10)
	_ = p.Text("страница 2")
	p.AddPage()
	p.SetX(10)
	p.SetY(10)
	_ = p.Text("страница 3")

	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	kids := regexp.MustCompile(`/Kids \[([^\]]*)\]`).FindAllSubmatch(data, -1)
	for _, k := range kids {
		t.Logf("kids: %q", k[1])
	}
	if len(kids) == 0 || len(regexp.MustCompile(`\d+ 0 R`).FindAll(kids[0][1], -1)) != 3 {
		t.Fatalf("expected 3 kids, got %q", kids)
	}
}

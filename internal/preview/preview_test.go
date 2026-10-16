package preview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestLoadKinds(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	book := excelize.NewFile()
	book.SetCellValue("Sheet1", "A1", "Имя")
	book.SetCellValue("Sheet1", "B2", 42)
	xlsx := filepath.Join(dir, "book.xlsx")
	if err := book.SaveAs(xlsx); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		write("notes.txt", []byte("hello")): KindText,
		write("photo.PNG", []byte{0x89}):    KindImage,
		write("scan.pdf", []byte("%PDF")):   KindPDF,
		write("data.bin", []byte{1, 2, 3}):  KindInfo,
		write("old.doc", []byte{0}):         KindInfo,
		xlsx:                                KindSheet,
	}
	for path, want := range cases {
		result, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if result.Kind != want {
			t.Errorf("%s: kind = %q, ожидалось %q", filepath.Base(path), result.Kind, want)
		}
	}

	result, _ := Load(xlsx)
	if got := result.Sheets[0].Rows; len(got) != 2 || got[0][0] != "Имя" || got[1][1] != "42" {
		t.Errorf("строки листа: %q", got)
	}
	if !Servable("a.PDF") || !Servable("b.jpg") || Servable("c.txt") {
		t.Error("Servable: неверно для pdf/jpg/txt")
	}
}

//go:build ignore

// Command gen_icon regenerates the application icon.
//
// It writes two files:
//
//	snap8.ico                  – a normal .ico file (edit/replace it freely)
//	rsrc_windows_amd64.syso    – a COFF resource object that `go build` links
//	                             into snap8.exe automatically, so the binary
//	                             shows the icon in Explorer / taskbar / Alt-Tab.
//
// Run after changing the artwork below:
//
//	go run gen_icon.go        (or: go generate ./...)
//
// Only the Go standard library is used — no external tools or modules. Note
// the icon images are stored as PNG, which Windows Vista+ supports inside both
// .ico files and icon resources; snap8 already targets Windows 10/11.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sort"
)

func main() {
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	imgs := make([]*image.NRGBA, 0, len(sizes))
	for _, s := range sizes {
		imgs = append(imgs, renderIcon(s))
	}
	if err := writeICO("snap8.ico", imgs); err != nil {
		fatal(err)
	}
	if err := writeSYSO("rsrc_windows_amd64.syso", imgs); err != nil {
		fatal(err)
	}
	fmt.Println("gen_icon: wrote snap8.ico and rsrc_windows_amd64.syso")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen_icon:", err)
	os.Exit(1)
}

// ---------------------------------------------------------------------------
// Artwork — eight light "window" tiles laid 2×4 on a blue panel, i.e. exactly
// what snap8 does to your windows. Tweak the colours / proportions to taste.
// ---------------------------------------------------------------------------

var (
	panelColor = color.NRGBA{R: 0x1E, G: 0x5F, B: 0xD8, A: 0xFF} // brand blue
	tileColor  = color.NRGBA{R: 0xF6, G: 0xF9, B: 0xFF, A: 0xFF} // near-white
)

func renderIcon(size int) *image.NRGBA {
	const ss = 4 // supersample for cheap anti-aliasing, then box-downscale
	big := size * ss
	hi := image.NewNRGBA(image.Rect(0, 0, big, big))
	drawArtwork(hi, big)
	return downsample(hi, ss)
}

func drawArtwork(img *image.NRGBA, size int) {
	const rows, cols = 2, 4

	// Rounded blue panel filling most of the canvas.
	bgM := iround(float64(size) * 0.035)
	bgR := iround(float64(size) * 0.16)
	fillRoundRect(img, bgM, bgM, size-bgM, size-bgM, bgR, panelColor)

	// 2×4 grid of tiles inside the panel.
	pad := iround(float64(size) * 0.13)
	gap := iround(float64(size) * 0.045)
	if gap < 1 {
		gap = 1
	}
	cellW := (size - 2*pad - (cols-1)*gap) / cols
	cellH := (size - 2*pad - (rows-1)*gap) / rows
	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}
	cellR := iround(float64(min(cellW, cellH)) * 0.18)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x0 := pad + c*(cellW+gap)
			y0 := pad + r*(cellH+gap)
			fillRoundRect(img, x0, y0, x0+cellW, y0+cellH, cellR, tileColor)
		}
	}
}

// fillRoundRect paints the rectangle [x0,x1)×[y0,y1) with rounded corners of
// radius rad (rad<=0 → plain rectangle) in colour col.
func fillRoundRect(img *image.NRGBA, x0, y0, x1, y1, rad int, col color.NRGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if insideRoundRect(x, y, x0, y0, x1, y1, rad) {
				img.SetNRGBA(x, y, col)
			}
		}
	}
}

func insideRoundRect(x, y, x0, y0, x1, y1, rad int) bool {
	if rad <= 0 {
		return true
	}
	// Snap (x,y) onto the nearest corner-arc centre. If a coordinate is not in
	// a corner band it isn't moved, so that axis contributes 0 to the test.
	cx, cy := x, y
	switch {
	case x < x0+rad:
		cx = x0 + rad
	case x >= x1-rad:
		cx = x1 - 1 - rad
	}
	switch {
	case y < y0+rad:
		cy = y0 + rad
	case y >= y1-rad:
		cy = y1 - 1 - rad
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= rad*rad
}

// downsample box-averages an ss×ss block of src into one pixel, working in
// premultiplied space so transparent edges stay clean.
func downsample(src *image.NRGBA, ss int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx()/ss, b.Dy()/ss
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := ss * ss
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rs, gs, bs, as int
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					p := src.NRGBAAt(x*ss+dx, y*ss+dy)
					a := int(p.A)
					rs += int(p.R) * a
					gs += int(p.G) * a
					bs += int(p.B) * a
					as += a
				}
			}
			out := color.NRGBA{A: uint8((as + n/2) / n)}
			if as > 0 {
				out.R = uint8((rs + as/2) / as)
				out.G = uint8((gs + as/2) / as)
				out.B = uint8((bs + as/2) / as)
			}
			dst.SetNRGBA(x, y, out)
		}
	}
	return dst
}

func iround(f float64) int     { return int(math.Round(f)) }
func align(v, a uint32) uint32 { return (v + a - 1) &^ (a - 1) }

// ---------------------------------------------------------------------------
// .ico file writer (PNG-encoded entries).
// ---------------------------------------------------------------------------

func writeICO(path string, imgs []*image.NRGBA) error {
	type entry struct {
		w, h int
		data []byte
	}
	entries := make([]entry, 0, len(imgs))
	for _, im := range imgs {
		var buf bytes.Buffer
		if err := png.Encode(&buf, im); err != nil {
			return err
		}
		bd := im.Bounds()
		entries = append(entries, entry{bd.Dx(), bd.Dy(), buf.Bytes()})
	}

	var out bytes.Buffer
	le := binary.LittleEndian
	put := func(v any) { _ = binary.Write(&out, le, v) }

	put(uint16(0))             // idReserved
	put(uint16(1))             // idType = 1 (icon)
	put(uint16(len(entries)))  // idCount
	off := 6 + 16*len(entries) // image data follows the directory
	for _, e := range entries {
		out.WriteByte(byte(e.w % 256)) // bWidth  (256 → 0)
		out.WriteByte(byte(e.h % 256)) // bHeight (256 → 0)
		out.WriteByte(0)               // bColorCount
		out.WriteByte(0)               // bReserved
		put(uint16(1))                 // wPlanes
		put(uint16(32))                // wBitCount
		put(uint32(len(e.data)))       // dwBytesInRes
		put(uint32(off))               // dwImageOffset
		off += len(e.data)
	}
	for _, e := range entries {
		out.Write(e.data)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// ---------------------------------------------------------------------------
// COFF (.syso) resource-object writer: one ".rsrc" section holding a 3-level
// RT_ICON / RT_GROUP_ICON directory, plus relocations turning each data
// entry's section-relative offset into an image RVA at link time.
// ---------------------------------------------------------------------------

func writeSYSO(path string, imgs []*image.NRGBA) error {
	const (
		rtIcon      = 3
		rtGroupIcon = 14
		langNeutral = 0
	)

	// RT_ICON resources 1..N (PNG bytes) + a GRPICONDIR tying them together.
	var iconPNG [][]byte
	var dimsW, dimsH []int
	for _, im := range imgs {
		var buf bytes.Buffer
		if err := png.Encode(&buf, im); err != nil {
			return err
		}
		iconPNG = append(iconPNG, buf.Bytes())
		bd := im.Bounds()
		dimsW = append(dimsW, bd.Dx())
		dimsH = append(dimsH, bd.Dy())
	}
	le := binary.LittleEndian
	var grp bytes.Buffer
	gput := func(v any) { _ = binary.Write(&grp, le, v) }
	gput(uint16(0))            // idReserved
	gput(uint16(1))            // idType = 1 (icon)
	gput(uint16(len(iconPNG))) // idCount
	for i := range iconPNG {
		grp.WriteByte(byte(dimsW[i] % 256)) // bWidth  (256 → 0)
		grp.WriteByte(byte(dimsH[i] % 256)) // bHeight (256 → 0)
		grp.WriteByte(0)                    // bColorCount
		grp.WriteByte(0)                    // bReserved
		gput(uint16(1))                     // wPlanes
		gput(uint16(32))                    // wBitCount
		gput(uint32(len(iconPNG[i])))       // dwBytesInRes
		gput(uint16(i + 1))                 // nID → RT_ICON resource id
	}

	// Flat list of resources, then grouped into the type→id→lang tree.
	type resource struct {
		typ, id, lang uint16
		data          []byte
	}
	var resources []resource
	for i, p := range iconPNG {
		resources = append(resources, resource{rtIcon, uint16(i + 1), langNeutral, p})
	}
	resources = append(resources, resource{rtGroupIcon, 1, langNeutral, grp.Bytes()})

	type leaf struct {
		res                resource
		dataEntOff, rawOff uint32
	}
	type idNode struct {
		id     uint16
		leaves []*leaf
	}
	type typeNode struct {
		typ uint16
		ids []*idNode
	}
	var types []*typeNode
	getType := func(t uint16) *typeNode {
		for _, tn := range types {
			if tn.typ == t {
				return tn
			}
		}
		tn := &typeNode{typ: t}
		types = append(types, tn)
		return tn
	}
	for _, r := range resources {
		tn := getType(r.typ)
		tn.ids = append(tn.ids, &idNode{id: r.id, leaves: []*leaf{{res: r}}})
	}
	// Directory entries must be sorted: by type id, then resource id, then lang.
	sort.SliceStable(types, func(i, j int) bool { return types[i].typ < types[j].typ })
	for _, tn := range types {
		sort.SliceStable(tn.ids, func(i, j int) bool { return tn.ids[i].id < tn.ids[j].id })
		for _, idn := range tn.ids {
			sort.SliceStable(idn.leaves, func(i, j int) bool { return idn.leaves[i].res.lang < idn.leaves[j].res.lang })
		}
	}

	const (
		dirHdrSize  = 16 // IMAGE_RESOURCE_DIRECTORY
		dirEntSize  = 8  // IMAGE_RESOURCE_DIRECTORY_ENTRY
		dataEntSize = 16 // IMAGE_RESOURCE_DATA_ENTRY
		subdirFlag  = uint32(1) << 31
	)

	// Pass 1 — assign every offset within the section.
	off := uint32(0)
	rootOff := off
	off += dirHdrSize + dirEntSize*uint32(len(types))
	typeDirOff := map[*typeNode]uint32{}
	for _, tn := range types {
		typeDirOff[tn] = off
		off += dirHdrSize + dirEntSize*uint32(len(tn.ids))
	}
	idDirOff := map[*idNode]uint32{}
	for _, tn := range types {
		for _, idn := range tn.ids {
			idDirOff[idn] = off
			off += dirHdrSize + dirEntSize*uint32(len(idn.leaves))
		}
	}
	for _, tn := range types {
		for _, idn := range tn.ids {
			for _, lf := range idn.leaves {
				lf.dataEntOff = off
				off += dataEntSize
			}
		}
	}
	for _, tn := range types {
		for _, idn := range tn.ids {
			for _, lf := range idn.leaves {
				off = align(off, 8)
				lf.rawOff = off
				off += uint32(len(lf.res.data))
			}
		}
	}
	rsrcSize := align(off, 8)

	// Pass 2 — emit the section bytes.
	sec := make([]byte, rsrcSize)
	putDirHdr := func(at uint32, nIDEntries int) {
		// Characteristics/TimeDateStamp/Major/Minor stay zero.
		le.PutUint16(sec[at+12:], 0)                  // NumberOfNamedEntries
		le.PutUint16(sec[at+14:], uint16(nIDEntries)) // NumberOfIdEntries
	}
	putDirEnt := func(at uint32, id uint16, target uint32, subdir bool) {
		le.PutUint32(sec[at:], uint32(id))
		if subdir {
			target |= subdirFlag
		}
		le.PutUint32(sec[at+4:], target)
	}

	putDirHdr(rootOff, len(types))
	ep := rootOff + dirHdrSize
	for _, tn := range types {
		putDirEnt(ep, tn.typ, typeDirOff[tn], true)
		ep += dirEntSize
	}
	for _, tn := range types {
		putDirHdr(typeDirOff[tn], len(tn.ids))
		ep := typeDirOff[tn] + dirHdrSize
		for _, idn := range tn.ids {
			putDirEnt(ep, idn.id, idDirOff[idn], true)
			ep += dirEntSize
		}
	}
	for _, tn := range types {
		for _, idn := range tn.ids {
			putDirHdr(idDirOff[idn], len(idn.leaves))
			ep := idDirOff[idn] + dirHdrSize
			for _, lf := range idn.leaves {
				putDirEnt(ep, lf.res.lang, lf.dataEntOff, false)
				ep += dirEntSize
			}
		}
	}
	var relocAt []uint32 // section offsets of OffsetToData fields needing a reloc
	for _, tn := range types {
		for _, idn := range tn.ids {
			for _, lf := range idn.leaves {
				at := lf.dataEntOff
				le.PutUint32(sec[at:], lf.rawOff)                  // OffsetToData (→ RVA via reloc)
				le.PutUint32(sec[at+4:], uint32(len(lf.res.data))) // Size
				le.PutUint32(sec[at+8:], 0)                        // CodePage
				le.PutUint32(sec[at+12:], 0)                       // Reserved
				relocAt = append(relocAt, at)
				copy(sec[lf.rawOff:], lf.res.data)
			}
		}
	}

	// Assemble: file header, section header, section data, relocations,
	// symbol table (".rsrc" section symbol + its aux record), string table.
	const (
		fileHdrSize        = 20
		sectHdrSize        = 40
		relocEntSize       = 10
		machineAMD64       = 0x8664
		scnCharacteristics = 0x40000040 // IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
		relAMD64Addr32NB   = 0x0003     // IMAGE_REL_AMD64_ADDR32NB (RVA)
		symClassStatic     = 3          // IMAGE_SYM_CLASS_STATIC
	)
	numRelocs := len(relocAt)
	const numSyms = 2 // section symbol + 1 aux section-definition record

	secRawAddr := uint32(fileHdrSize + sectHdrSize)
	relocAddr := secRawAddr + rsrcSize
	symTabAddr := relocAddr + uint32(numRelocs*relocEntSize)

	var out bytes.Buffer
	w16 := func(v uint16) { _ = binary.Write(&out, le, v) }
	w32 := func(v uint32) { _ = binary.Write(&out, le, v) }

	// IMAGE_FILE_HEADER
	w16(machineAMD64)
	w16(1) // NumberOfSections
	w32(0) // TimeDateStamp
	w32(symTabAddr)
	w32(numSyms)
	w16(0) // SizeOfOptionalHeader
	w16(0) // Characteristics

	// IMAGE_SECTION_HEADER ".rsrc"
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	w32(0)          // VirtualSize
	w32(0)          // VirtualAddress
	w32(rsrcSize)   // SizeOfRawData
	w32(secRawAddr) // PointerToRawData
	if numRelocs > 0 {
		w32(relocAddr) // PointerToRelocations
	} else {
		w32(0)
	}
	w32(0)                  // PointerToLinenumbers
	w16(uint16(numRelocs))  // NumberOfRelocations
	w16(0)                  // NumberOfLinenumbers
	w32(scnCharacteristics) // Characteristics

	out.Write(sec)

	for _, ra := range relocAt {
		w32(ra)               // VirtualAddress (offset of the OffsetToData field)
		w32(0)                // SymbolTableIndex → ".rsrc" symbol (index 0)
		w16(relAMD64Addr32NB) // Type
	}

	// Symbol 0: ".rsrc" section symbol.
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0}) // Name (fits in 8 bytes)
	w32(0)                                              // Value
	w16(1)                                              // SectionNumber (1-based)
	w16(0)                                              // Type
	out.WriteByte(symClassStatic)                       // StorageClass
	out.WriteByte(1)                                    // NumberOfAuxSymbols
	// Symbol 1: aux section definition (18 bytes).
	w32(rsrcSize)              // Length
	w16(uint16(numRelocs))     // NumberOfRelocations
	w16(0)                     // NumberOfLinenumbers
	w32(0)                     // CheckSum
	w16(0)                     // Number (not a COMDAT section)
	out.WriteByte(0)           // Selection
	out.Write([]byte{0, 0, 0}) // padding to 18 bytes

	// COFF string table — mandatory; here just the 4-byte size field (empty).
	w32(4)

	return os.WriteFile(path, out.Bytes(), 0o644)
}

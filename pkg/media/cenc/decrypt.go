package cenc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"os"
)

// Box represents an MP4 ISO base media file format box.
type Box struct {
	Typ      string
	Off      int
	Size     int
	Hdr      int
	Children []Box
}

// ParseBoxes recursively parses MP4 boxes within data[start:end].
func ParseBoxes(data []byte, start, end int) []Box {
	var boxes []Box
	off := start

	for off+8 <= end {
		size := int(binary.BigEndian.Uint32(data[off : off+4]))
		typ := string(data[off+4 : off+8])
		hdr := 8

		if size == 1 {
			if off+16 > end {
				break
			}
			size = int(binary.BigEndian.Uint64(data[off+8 : off+16]))
			hdr = 16
		} else if size == 0 {
			size = end - off
		}

		if size < hdr || off+size > end {
			break
		}

		var children []Box
		switch typ {
		case "moov", "trak", "mdia", "minf", "stbl", "edts", "dinf", "udta", "meta":
			children = ParseBoxes(data, off+hdr, off+size)
		}

		boxes = append(boxes, Box{
			Typ:      typ,
			Off:      off,
			Size:     size,
			Hdr:      hdr,
			Children: children,
		})
		off += size
	}

	return boxes
}

func cloneBox(b Box) Box {
	var children []Box
	if b.Children != nil {
		children = make([]Box, len(b.Children))
		for i, c := range b.Children {
			children[i] = cloneBox(c)
		}
	}
	return Box{
		Typ:      b.Typ,
		Off:      b.Off,
		Size:     b.Size,
		Hdr:      b.Hdr,
		Children: children,
	}
}

func pruneBox(b Box) Box {
	if b.Children == nil {
		return b
	}
	var keep []Box
	for _, c := range b.Children {
		switch c.Typ {
		case "senc", "saio", "saiz", "sgpd", "sbgp":
			continue
		default:
			keep = append(keep, pruneBox(c))
		}
	}
	b.Children = keep
	return b
}

func walkBoxes(boxes []Box, fn func(b *Box)) {
	for i := range boxes {
		fn(&boxes[i])
		if len(boxes[i].Children) > 0 {
			walkBoxes(boxes[i].Children, fn)
		}
	}
}

// RebuildMoov strips encryption metadata (senc, saio, saiz) from moov,
// restores encv/enca back to avc1/hvc1/mp4a, adjusts stco chunk offsets, and serializes the new moov box.
func RebuildMoov(data []byte, moovOff, moovSize int, top []Box) ([]byte, error) {
	var moovBox *Box
	for i := range top {
		if top[i].Typ == "moov" {
			moovBox = &top[i]
			break
		}
	}
	if moovBox == nil {
		return nil, fmt.Errorf("no moov box found")
	}

	tree := pruneBox(cloneBox(*moovBox))
	patches := make(map[int][]byte)

	var stsdErr error
	walkBoxes([]Box{tree}, func(b *Box) {
		if b.Typ != "stsd" || stsdErr != nil {
			return
		}
		content := b.Off + 8
		if content+8 > len(data) {
			stsdErr = fmt.Errorf("stsd box out of bounds")
			return
		}
		count := binary.BigEndian.Uint32(data[content+4 : content+8])
		p := content + 8
		var entries [][]byte

		for i := 0; i < int(count); i++ {
			if p+8 > len(data) {
				stsdErr = fmt.Errorf("stsd entry out of bounds")
				return
			}
			esize := int(binary.BigEndian.Uint32(data[p : p+4]))
			if esize < 8 || p+esize > len(data) {
				stsdErr = fmt.Errorf("invalid stsd entry size")
				return
			}
			etyp := string(data[p+4 : p+8])

			if etyp == "encv" || etyp == "enca" {
				hdrSize := 28
				if etyp == "encv" {
					hdrSize = 78
				}
				if p+8+hdrSize > p+esize {
					stsdErr = fmt.Errorf("encv/enca entry too short")
					return
				}
				children := ParseBoxes(data, p+8+hdrSize, p+esize)
				newType := "mp4a"

				if etyp == "encv" {
					var sinf *Box
					for j := range children {
						if children[j].Typ == "sinf" {
							sinf = &children[j]
							break
						}
					}
					var original string
					if sinf != nil {
						sinfChildren := ParseBoxes(data, sinf.Off+sinf.Hdr, sinf.Off+sinf.Size)
						for _, sc := range sinfChildren {
							if sc.Typ == "frma" && sc.Size == sc.Hdr+4 && sc.Off+sc.Hdr+4 <= len(data) {
								original = string(data[sc.Off+sc.Hdr : sc.Off+sc.Hdr+4])
								break
							}
						}
					}
					avc := false
					hevc := false
					for _, c := range children {
						if c.Typ == "avcC" {
							avc = true
						}
						if c.Typ == "hvcC" {
							hevc = true
						}
					}
					if original != "" {
						newType = original
					} else if avc && !hevc {
						newType = "avc1"
					} else if hevc && !avc {
						newType = "hvc1"
					} else {
						stsdErr = fmt.Errorf("unable to determine codec for encv")
						return
					}
				}

				entry := make([]byte, 8+hdrSize)
				copy(entry[4:8], []byte(newType))
				copy(entry[8:], data[p+8:p+8+hdrSize])

				var entryExtra []byte
				for _, child := range children {
					if child.Typ != "sinf" {
						entryExtra = append(entryExtra, data[child.Off:child.Off+child.Size]...)
					}
				}

				fullEntry := append(entry, entryExtra...)
				binary.BigEndian.PutUint32(fullEntry[0:4], uint32(len(fullEntry)))
				entries = append(entries, fullEntry)
			} else {
				entryCopy := make([]byte, esize)
				copy(entryCopy, data[p:p+esize])
				entries = append(entries, entryCopy)
			}
			p += esize
		}

		stsdHdr := make([]byte, 16)
		copy(stsdHdr, data[b.Off:content+8])
		binary.BigEndian.PutUint32(stsdHdr[12:16], uint32(len(entries)))

		var fullStsd []byte
		fullStsd = append(fullStsd, stsdHdr...)
		for _, e := range entries {
			fullStsd = append(fullStsd, e...)
		}
		binary.BigEndian.PutUint32(fullStsd[0:4], uint32(len(fullStsd)))
		patches[b.Off] = fullStsd
	})

	if stsdErr != nil {
		return nil, stsdErr
	}

	var serialize func(box Box) []byte
	serialize = func(box Box) []byte {
		if box.Children == nil {
			if patch, ok := patches[box.Off]; ok {
				return patch
			}
			res := make([]byte, box.Size)
			copy(res, data[box.Off:box.Off+box.Size])
			return res
		}
		var body []byte
		for _, c := range box.Children {
			body = append(body, serialize(c)...)
		}
		hdrb := make([]byte, box.Hdr)
		copy(hdrb, data[box.Off:box.Off+box.Hdr])
		if box.Hdr == 8 {
			binary.BigEndian.PutUint32(hdrb[0:4], uint32(8+len(body)))
		} else {
			binary.BigEndian.PutUint32(hdrb[0:4], 1)
			binary.BigEndian.PutUint64(hdrb[8:16], uint64(16+len(body)))
		}
		return append(hdrb, body...)
	}

	first := serialize(tree)
	delta := moovSize - len(first)

	// Adjust stco chunk offsets if moov size changed
	walkBoxes([]Box{tree}, func(b *Box) {
		if b.Typ == "stco" {
			if b.Off+16 > len(data) {
				return
			}
			nc := int(binary.BigEndian.Uint32(data[b.Off+12 : b.Off+16]))
			for i := 0; i < nc; i++ {
				pos := b.Off + 16 + i*4
				if pos+4 > len(data) {
					break
				}
				v := binary.BigEndian.Uint32(data[pos : pos+4])
				if int(v) >= moovOff+moovSize {
					binary.BigEndian.PutUint32(data[pos:pos+4], v-uint32(delta))
				}
			}
		}
	})

	return serialize(tree), nil
}

// DecryptSample decrypts a single sample using AES-128-CTR with the 8-byte nonce and 64-bit counter.
func DecryptSample(key, nonce, cipherBytes []byte) ([]byte, error) {
	if len(cipherBytes) == 0 {
		return []byte{}, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher failed: %w", err)
	}

	iv := make([]byte, 16)
	copy(iv[:8], nonce) // first 8 bytes nonce, remaining 8 bytes counter starting at 0

	plain := make([]byte, len(cipherBytes))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(plain, cipherBytes)
	return plain, nil
}

type trackSampleInfo struct {
	sizes []int
	offs  []int
	ivs   [][]byte
}

// DecryptMp4Buffer decrypts a CENC-AES-CTR encrypted MP4 in memory and returns clear MP4 bytes.
func DecryptMp4Buffer(data []byte, key []byte) ([]byte, error) {
	total := len(data)
	top := ParseBoxes(data, 0, total)

	var moov *Box
	for i := range top {
		if top[i].Typ == "moov" {
			moov = &top[i]
			break
		}
	}
	if moov == nil {
		return nil, fmt.Errorf("no moov box found in MP4")
	}

	var stbls []*Box
	walkBoxes(top, func(b *Box) {
		if b.Typ == "stbl" {
			stbls = append(stbls, b)
		}
	})

	findChild := func(b *Box, typ string) *Box {
		for i := range b.Children {
			if b.Children[i].Typ == typ {
				return &b.Children[i]
			}
		}
		return nil
	}

	var trackInfos []trackSampleInfo
	for _, stbl := range stbls {
		stsz := findChild(stbl, "stsz")
		stco := findChild(stbl, "stco")
		stsc := findChild(stbl, "stsc")
		senc := findChild(stbl, "senc")

		if stsz == nil || stco == nil || stsc == nil {
			continue
		}

		if stsz.Off+20 > total || stco.Off+16 > total || stsc.Off+16 > total {
			continue
		}

		ss := int(binary.BigEndian.Uint32(data[stsz.Off+12 : stsz.Off+16]))
		n := int(binary.BigEndian.Uint32(data[stsz.Off+16 : stsz.Off+20]))
		sizes := make([]int, n)
		if ss == 0 {
			for i := 0; i < n; i++ {
				pos := stsz.Off + 20 + i*4
				if pos+4 <= total {
					sizes[i] = int(binary.BigEndian.Uint32(data[pos : pos+4]))
				}
			}
		} else {
			for i := 0; i < n; i++ {
				sizes[i] = ss
			}
		}

		nc := int(binary.BigEndian.Uint32(data[stco.Off+12 : stco.Off+16]))
		chunkOffs := make([]int, nc)
		for i := 0; i < nc; i++ {
			pos := stco.Off + 16 + i*4
			if pos+4 <= total {
				chunkOffs[i] = int(binary.BigEndian.Uint32(data[pos : pos+4]))
			}
		}

		ns := int(binary.BigEndian.Uint32(data[stsc.Off+12 : stsc.Off+16]))
		type stscEntry struct {
			firstChunk int
			spc        int
			sdi        int
		}
		stscTab := make([]stscEntry, ns)
		for i := 0; i < ns; i++ {
			pos := stsc.Off + 16 + i*12
			if pos+12 <= total {
				stscTab[i] = stscEntry{
					firstChunk: int(binary.BigEndian.Uint32(data[pos : pos+4])),
					spc:        int(binary.BigEndian.Uint32(data[pos+4 : pos+8])),
					sdi:        int(binary.BigEndian.Uint32(data[pos+8 : pos+12])),
				}
			}
		}

		var ivs [][]byte
		if senc != nil && senc.Off+16 <= total {
			sc := int(binary.BigEndian.Uint32(data[senc.Off+12 : senc.Off+16]))
			for i := 0; i < sc; i++ {
				pos := senc.Off + 16 + i*8
				if pos+8 <= total {
					iv := make([]byte, 8)
					copy(iv, data[pos:pos+8])
					ivs = append(ivs, iv)
				}
			}
		}

		chunkSpc := make(map[int]int)
		for i := 0; i < len(stscTab); i++ {
			fc := stscTab[i].firstChunk
			spc := stscTab[i].spc
			nxt := nc + 1
			if i+1 < len(stscTab) {
				nxt = stscTab[i+1].firstChunk
			}
			for ci := fc - 1; ci < nxt-1; ci++ {
				chunkSpc[ci] = spc
			}
		}

		var sampleOffs []int
		si := 0
		for ci := 0; ci < nc; ci++ {
			off := chunkOffs[ci]
			spc := chunkSpc[ci]
			if spc == 0 {
				spc = 1
			}
			for s := 0; s < spc; s++ {
				if si >= n {
					break
				}
				sampleOffs = append(sampleOffs, off)
				off += sizes[si]
				si++
			}
		}

		trackInfos = append(trackInfos, trackSampleInfo{
			sizes: sizes,
			offs:  sampleOffs,
			ivs:   ivs,
		})
	}

	// In-place decrypt samples
	for _, ti := range trackInfos {
		for i := 0; i < len(ti.offs); i++ {
			off := ti.offs[i]
			sz := ti.sizes[i]
			if off+sz > total {
				continue
			}
			if i < len(ti.ivs) {
				nonce := ti.ivs[i]
				plain, err := DecryptSample(key, nonce, data[off:off+sz])
				if err != nil {
					return nil, err
				}
				copy(data[off:], plain)
			}
		}
	}

	newMoov, err := RebuildMoov(data, moov.Off, moov.Size, top)
	if err != nil {
		return nil, fmt.Errorf("rebuildMoov failed: %w", err)
	}

	var out bytes.Buffer
	off := 0
	for off < total {
		if off+8 > total {
			out.Write(data[off:])
			break
		}
		size := int(binary.BigEndian.Uint32(data[off : off+4]))
		typ := string(data[off+4 : off+8])
		hdr := 8
		if size == 1 {
			size = int(binary.BigEndian.Uint64(data[off+8 : off+16]))
			hdr = 16
		} else if size == 0 {
			size = total - off
		}
		if size < hdr || off+size > total {
			out.Write(data[off:])
			break
		}

		if typ == "moov" {
			out.Write(newMoov)
		} else {
			out.Write(data[off : off+size])
		}
		off += size
	}

	return out.Bytes(), nil
}

// DecryptMp4File reads srcPath, decrypts in memory, and writes to dstPath.
func DecryptMp4File(srcPath, dstPath string, key []byte) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	decrypted, err := DecryptMp4Buffer(data, key)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, decrypted, 0644)
}

// SPDX-License-Identifier: MIT

package avifcodec

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type box struct {
	kind string
	data []byte
}

// boxes walks only a single level and never trusts a box length. Limits also
// bound work on a file made of millions of tiny metadata records.
func boxes(data []byte) ([]box, error) {
	var out []box
	for len(data) > 0 {
		if len(data) < 8 || len(out) >= 8192 {
			return nil, errors.New("invalid AVIF boxes")
		}
		size := uint64(binary.BigEndian.Uint32(data))
		hdr := uint64(8)
		if size == 1 {
			if len(data) < 16 {
				return nil, errors.New("short AVIF box")
			}
			size = binary.BigEndian.Uint64(data[8:])
			hdr = 16
		}
		if size == 0 {
			size = uint64(len(data))
		}
		if size < hdr || size > uint64(len(data)) {
			return nil, errors.New("invalid AVIF box size")
		}
		out = append(out, box{string(data[4:8]), data[hdr:size]})
		data = data[size:]
	}
	return out, nil
}

func child(data []byte, kind string) ([]byte, error) {
	bs, err := boxes(data)
	if err != nil {
		return nil, err
	}
	var result []byte
	for _, b := range bs {
		if b.kind == kind {
			if result != nil {
				return nil, errors.New("duplicate AVIF metadata box")
			}
			result = b.data
		}
	}
	if result == nil {
		return nil, fmt.Errorf("missing AVIF %s box", kind)
	}
	return result, nil
}

// inspectProperties handles primary-item orientation, and refuses features
// that the RGB bridge would silently discard or render with wrong colours.
// HDR/WCG/ICC and clean-aperture cropping need a colour/crop pipeline first.
func inspectProperties(data []byte) (transform, error) {
	t := transform{mirror: -1}
	top, err := boxes(data)
	if err != nil {
		return t, err
	}
	for _, b := range top {
		if b.kind == "moov" {
			return t, fmt.Errorf("%w: image sequences", ErrUnsupported)
		}
	}
	meta, err := child(data, "meta")
	if err != nil {
		return t, err
	}
	if len(meta) < 4 || meta[0] != 0 {
		return t, errors.New("invalid AVIF meta")
	}
	meta = meta[4:]
	pitm, err := child(meta, "pitm")
	if err != nil {
		return t, err
	}
	if len(pitm) < 6 {
		return t, errors.New("invalid AVIF primary item")
	}
	primary := uint32(binary.BigEndian.Uint16(pitm[4:]))
	if pitm[0] == 1 {
		if len(pitm) < 8 {
			return t, errors.New("short AVIF primary item")
		}
		primary = binary.BigEndian.Uint32(pitm[4:])
	} else if pitm[0] != 0 {
		return t, errors.New("invalid AVIF primary item version")
	}
	iprp, err := child(meta, "iprp")
	if err != nil {
		return t, err
	}
	ipco, err := child(iprp, "ipco")
	if err != nil {
		return t, err
	}
	props, err := boxes(ipco)
	if err != nil {
		return t, err
	}
	// Check every item's colour/crop properties, including grid tiles, rather
	// than accepting a harmless primary property hiding an unsupported tile.
	for _, p := range props {
		switch p.kind {
		case "clap":
			return t, fmt.Errorf("%w: clean aperture crop", ErrUnsupported)
		case "colr":
			if len(p.data) < 4 {
				return t, errors.New("short AVIF colour property")
			}
			if string(p.data[:4]) != "nclx" || len(p.data) != 11 {
				return t, fmt.Errorf("%w: ICC/unknown colour profile", ErrUnsupported)
			}
			prim := binary.BigEndian.Uint16(p.data[4:])
			transfer := binary.BigEndian.Uint16(p.data[6:])
			// Unspecified or BT.709 primaries; SDR transfer functions only. libavif
			// performs the YUV matrix conversion, but not HDR tone/gamut mapping.
			if (prim != 1 && prim != 2) || (transfer != 1 && transfer != 2 && transfer != 6 && transfer != 13) {
				return t, fmt.Errorf("%w: HDR or wide-gamut colour", ErrUnsupported)
			}
		}
	}
	ipma, err := child(iprp, "ipma")
	if err != nil {
		return t, err
	}
	if len(ipma) < 8 || ipma[0] > 1 {
		return t, errors.New("invalid AVIF property associations")
	}
	wide := ipma[3]&1 != 0
	version := ipma[0]
	count := binary.BigEndian.Uint32(ipma[4:])
	pos := 8
	if count > 8192 {
		return t, errors.New("too many AVIF items")
	}
	seen := false
	rotSeen, mirSeen := false, false
	for i := uint32(0); i < count; i++ {
		idBytes := 2
		if version == 1 {
			idBytes = 4
		}
		if pos+idBytes+1 > len(ipma) {
			return t, errors.New("short AVIF associations")
		}
		id := uint32(binary.BigEndian.Uint16(ipma[pos:]))
		if idBytes == 4 {
			id = binary.BigEndian.Uint32(ipma[pos:])
		}
		pos += idBytes
		n := int(ipma[pos])
		pos++
		if id == primary {
			if seen {
				return t, errors.New("duplicate primary AVIF association")
			}
			seen = true
		}
		for j := 0; j < n; j++ {
			idxBytes := 1
			if wide {
				idxBytes = 2
			}
			if pos+idxBytes > len(ipma) {
				return t, errors.New("short AVIF property index")
			}
			idx := int(ipma[pos] & 0x7f)
			if wide {
				idx = int(binary.BigEndian.Uint16(ipma[pos:]) & 0x7fff)
			}
			pos += idxBytes
			if idx == 0 {
				continue
			}
			if idx > len(props) {
				return t, errors.New("invalid AVIF property index")
			}
			if id != primary {
				continue
			}
			p := props[idx-1]
			switch p.kind {
			case "irot":
				if rotSeen || len(p.data) != 1 || p.data[0] > 3 {
					return t, errors.New("invalid AVIF rotation")
				}
				rotSeen = true
				t.angle = int(p.data[0])
			case "imir":
				if mirSeen || len(p.data) != 1 || p.data[0] > 1 {
					return t, errors.New("invalid AVIF mirror")
				}
				mirSeen = true
				t.mirror = int(p.data[0])
			}
		}
	}
	if !seen || pos != len(ipma) {
		return t, errors.New("invalid AVIF primary associations")
	}
	return t, nil
}

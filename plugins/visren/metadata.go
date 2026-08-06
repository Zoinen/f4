package visren

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/unxed/f4/vfs"
	_ "golang.org/x/image/bmp"
)

func readMetadata(path string, mtime time.Time) (Metadata, error) {
	var meta Metadata
	ext := strings.ToLower(filepath.Ext(path))
	var errs []string
	if ext == ".mp3" {
		m, err := readID3(path)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			meta = m
		}
	}
	if ext == ".jpg" || ext == ".jpeg" || ext == ".bmp" || ext == ".gif" || ext == ".png" {
		m, err := readImageMetadata(path, mtime)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			meta.CameraMake, meta.CameraModel = m.CameraMake, m.CameraModel
			meta.ImageDate, meta.Width, meta.Height = m.ImageDate, m.Width, m.Height
		}
	}
	if ext == ".exe" || ext == ".dll" {
		meta.Version, _ = readPEVersion(path)
	}
	if len(errs) > 0 {
		return meta, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return meta, nil
}

func readID3(path string) (Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Metadata{}, err
	}

	var v1 Metadata
	if stat.Size() >= 128 {
		buf := make([]byte, 128)
		if _, err := f.ReadAt(buf, stat.Size()-128); err == nil && string(buf[:3]) == "TAG" {
			v1.Title = decodeLegacy(buf[3:33])
			v1.Artist = decodeLegacy(buf[33:63])
			v1.Album = decodeLegacy(buf[63:93])
			v1.Year = decodeLegacy(buf[93:97])
			if buf[125] == 0 && buf[126] != 0 {
				v1.Track = strconv.Itoa(int(buf[126]))
			}
			if int(buf[127]) < len(id3Genres) {
				v1.Genre = id3Genres[int(buf[127])]
			}
		}
	}

	header := make([]byte, 10)
	if _, err := f.ReadAt(header, 0); err != nil || string(header[:3]) != "ID3" {
		return v1, nil
	}
	version := header[3]
	if version < 3 || version > 4 {
		return v1, nil
	}
	size := syncSafe(header[6:10])
	if size <= 0 || size > 32<<20 {
		return v1, nil
	}
	data := make([]byte, size)
	if _, err := f.ReadAt(data, 10); err != nil && err != io.EOF {
		return v1, err
	}
	if header[5]&0x80 != 0 {
		data = removeUnsynchronisation(data)
	}
	if header[5]&0x40 != 0 && len(data) >= 4 {
		extSize := int(binary.BigEndian.Uint32(data[:4]))
		if version == 4 {
			extSize = syncSafe(data[:4])
		}
		if extSize > 0 && extSize <= len(data) {
			data = data[extSize:]
		}
	}

	// A present ID3v2 tag is authoritative, matching VisRen.
	result := Metadata{}
	for len(data) >= 10 && data[0] != 0 {
		id := string(data[:4])
		frameSize := int(binary.BigEndian.Uint32(data[4:8]))
		if version == 4 {
			frameSize = syncSafe(data[4:8])
		}
		if frameSize <= 0 || 10+frameSize > len(data) {
			break
		}
		text := decodeID3Text(data[10 : 10+frameSize])
		switch id {
		case "TRCK":
			result.Track = strings.SplitN(text, "/", 2)[0]
		case "TIT2":
			result.Title = text
		case "TPE1":
			result.Artist = text
		case "TALB":
			result.Album = text
		case "TYER", "TDRC":
			result.Year = text
		case "TCON":
			result.Genre = decodeGenre(text)
		}
		data = data[10+frameSize:]
	}
	return result, nil
}

func decodeLegacy(data []byte) string {
	data = bytes.TrimRight(data, "\x00 ")
	if len(data) == 0 {
		return ""
	}
	decoded, err := vfs.GetSystemANSIEncoding().NewDecoder().Bytes(data)
	if err != nil {
		return string(data)
	}
	return strings.TrimSpace(string(decoded))
}

func decodeID3Text(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	encoding := data[0]
	data = data[1:]
	switch encoding {
	case 0:
		return decodeLegacy(data)
	case 3:
		return strings.Trim(strings.TrimSpace(string(data)), "\x00")
	case 1, 2:
		little := encoding == 1
		if len(data) >= 2 && encoding == 1 {
			if data[0] == 0xfe && data[1] == 0xff {
				little, data = false, data[2:]
			} else if data[0] == 0xff && data[1] == 0xfe {
				little, data = true, data[2:]
			}
		}
		words := make([]uint16, 0, len(data)/2)
		for len(data) >= 2 {
			var w uint16
			if little {
				w = binary.LittleEndian.Uint16(data)
			} else {
				w = binary.BigEndian.Uint16(data)
			}
			if w == 0 {
				break
			}
			words = append(words, w)
			data = data[2:]
		}
		return strings.TrimSpace(string(utf16.Decode(words)))
	default:
		return ""
	}
}

func syncSafe(data []byte) int {
	if len(data) < 4 {
		return 0
	}
	return int(data[0]&0x7f)<<21 | int(data[1]&0x7f)<<14 | int(data[2]&0x7f)<<7 | int(data[3]&0x7f)
}

func removeUnsynchronisation(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		out = append(out, data[i])
		if data[i] == 0xff && i+1 < len(data) && data[i+1] == 0 {
			i++
		}
	}
	return out
}

func decodeGenre(text string) string {
	text = strings.TrimSpace(text)
	if n, err := strconv.Atoi(text); err == nil && n >= 0 && n < len(id3Genres) {
		return id3Genres[n]
	}
	if strings.HasPrefix(text, "(") {
		if end := strings.IndexByte(text, ')'); end > 1 {
			if n, err := strconv.Atoi(text[1:end]); err == nil && n >= 0 && n < len(id3Genres) {
				if end+1 == len(text) {
					return id3Genres[n]
				}
				return strings.TrimSpace(text[end+1:])
			}
		}
	}
	return text
}

var id3Genres = []string{
	"Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge", "Hip-Hop", "Jazz", "Metal",
	"New Age", "Oldies", "Other", "Pop", "R&B", "Rap", "Reggae", "Rock", "Techno", "Industrial",
	"Alternative", "Ska", "Death Metal", "Pranks", "Soundtrack", "Euro-Techno", "Ambient", "Trip-Hop", "Vocal", "Jazz & Funk",
	"Fusion", "Trance", "Classical", "Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel", "Noise",
	"Alt Rock", "Bass", "Soul", "Punk", "Space", "Meditative", "Instrumental Pop", "Instrumental Rock", "Ethnic", "Gothic",
	"Darkwave", "Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance", "Dream", "Southern Rock", "Comedy", "Cult", "Gangsta Rap",
	"Top 40", "Christian Rap", "Pop/Funk", "Jungle", "Native American", "Cabaret", "New Wave", "Psychedelic", "Rave", "Showtunes",
	"Trailer", "Lo-Fi", "Tribal", "Acid Punk", "Acid Jazz", "Polka", "Retro", "Musical", "Rock & Roll", "Hard Rock",
	"Folk", "Folk-Rock", "National Folk", "Swing", "Fast-Fusion", "Bebob", "Latin", "Revival", "Celtic", "Bluegrass",
	"Avantgarde", "Gothic Rock", "Progressive Rock", "Psychedelic Rock", "Symphonic Rock", "Slow Rock", "Big Band", "Chorus", "Easy Listening", "Acoustic",
	"Humour", "Speech", "Chanson", "Opera", "Chamber Music", "Sonata", "Symphony", "Booty Bass", "Primus", "Porn Groove",
	"Satire", "Slow Jam", "Club", "Tango", "Samba", "Folklore", "Ballad", "Power Ballad", "Rhythmic Soul", "Freestyle",
	"Duet", "Punk Rock", "Drum Solo", "A Cappella", "Euro-House", "Dance Hall", "Goa", "Drum & Bass", "Club-House", "Hardcore",
	"Terror", "Indie", "BritPop", "Negerpunk", "Polsk Punk", "Beat", "Christian Gangsta Rap", "Heavy Metal", "Black Metal", "Crossover",
	"Contemporary Christian", "Christian Rock", "Merengue", "Salsa", "Thrash Metal", "Anime", "JPop", "Synthpop",
}

func readImageMetadata(path string, mtime time.Time) (Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return Metadata{}, err
	}
	meta := Metadata{Width: cfg.Width, Height: cfg.Height, ImageDate: mtime.Local().Format("2006.01.02 15-04-05")}
	if strings.EqualFold(filepath.Ext(path), ".jpg") || strings.EqualFold(filepath.Ext(path), ".jpeg") {
		if data, err := os.ReadFile(path); err == nil {
			make_, model, date := parseJPEGEXIF(data)
			meta.CameraMake, meta.CameraModel = make_, model
			if date != "" {
				meta.ImageDate = date
			}
		}
	}
	return meta, nil
}

func parseJPEGEXIF(data []byte) (string, string, string) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return "", "", ""
	}
	for pos := 2; pos+4 <= len(data); {
		if data[pos] != 0xff {
			break
		}
		marker := data[pos+1]
		pos += 2
		if marker == 0xd9 || marker == 0xda || pos+2 > len(data) {
			break
		}
		size := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if size < 2 || pos+size > len(data) {
			break
		}
		section := data[pos+2 : pos+size]
		if marker == 0xe1 && len(section) >= 6 && string(section[:6]) == "Exif\x00\x00" {
			return parseTIFF(section[6:])
		}
		pos += size
	}
	return "", "", ""
}

func parseTIFF(data []byte) (string, string, string) {
	if len(data) < 8 {
		return "", "", ""
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return "", "", ""
	}
	if order.Uint16(data[2:4]) != 42 {
		return "", "", ""
	}
	readIFD := func(offset uint32) (map[uint16]string, uint32) {
		values := make(map[uint16]string)
		var nested uint32
		if int(offset)+2 > len(data) {
			return values, 0
		}
		count := int(order.Uint16(data[offset : offset+2]))
		for n := 0; n < count; n++ {
			p := int(offset) + 2 + n*12
			if p+12 > len(data) {
				break
			}
			tag, typ := order.Uint16(data[p:p+2]), order.Uint16(data[p+2:p+4])
			length := order.Uint32(data[p+4 : p+8])
			if tag == 0x8769 && typ == 4 && length == 1 {
				nested = order.Uint32(data[p+8 : p+12])
				continue
			}
			if typ != 2 || length == 0 {
				continue
			}
			var raw []byte
			if length <= 4 {
				raw = data[p+8 : p+8+int(length)]
			} else {
				off := int(order.Uint32(data[p+8 : p+12]))
				if off < 0 || off+int(length) > len(data) {
					continue
				}
				raw = data[off : off+int(length)]
			}
			values[tag] = strings.TrimSpace(strings.TrimRight(string(raw), "\x00"))
		}
		return values, nested
	}
	values, nested := readIFD(order.Uint32(data[4:8]))
	if nested != 0 {
		exifValues, _ := readIFD(nested)
		for tag, value := range exifValues {
			values[tag] = value
		}
	}
	date := values[0x9003]
	if date == "" {
		date = values[0x9004]
	}
	if date == "" {
		date = values[0x0132]
	}
	if len(date) >= 19 {
		date = date[:4] + "." + date[5:7] + "." + date[8:10] + " " + date[11:13] + "-" + date[14:16] + "-" + date[17:19]
	}
	return values[0x010f], values[0x0110], date
}

func readPEVersion(path string) (string, error) {
	f, err := pe.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	for _, section := range f.Sections {
		if section.Name != ".rsrc" {
			continue
		}
		data, err := section.Data()
		if err != nil {
			return "", err
		}
		return fixedFileVersion(data), nil
	}
	return "", nil
}

func fixedFileVersion(data []byte) string {
	idx := bytes.Index(data, []byte{0xbd, 0x04, 0xef, 0xfe})
	if idx < 0 || idx+16 > len(data) {
		return ""
	}
	ms := binary.LittleEndian.Uint32(data[idx+8 : idx+12])
	ls := binary.LittleEndian.Uint32(data[idx+12 : idx+16])
	parts := []uint16{uint16(ms >> 16), uint16(ms), uint16(ls >> 16), uint16(ls)}
	if parts[0]|parts[1]|parts[2]|parts[3] == 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", parts[0], parts[1], parts[2], parts[3])
}

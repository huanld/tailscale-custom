package main

// Simple embedded icons as ICO-format byte arrays.
// These are minimal 16x16 icons for the system tray.

// iconConnected is a green circle icon (connected state)
var iconConnected = generateIcon(0x00, 0xAA, 0x55) // green

// iconDisconnected is a gray circle icon (disconnected state)
var iconDisconnected = generateIcon(0x88, 0x88, 0x88) // gray

// generateIcon creates a minimal 16x16 ICO file with a solid circle of the given color.
func generateIcon(r, g, b byte) []byte {
	const size = 16

	// BMP data (32-bit BGRA, bottom-up)
	pixels := make([]byte, size*size*4)
	cx, cy := float64(size)/2.0, float64(size)/2.0
	radius := float64(size)/2.0 - 1

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// bottom-up: row 0 is the bottom
			row := size - 1 - y
			idx := (row*size + x) * 4
			dx := float64(x) - cx + 0.5
			dy := float64(y) - cy + 0.5
			dist := dx*dx + dy*dy
			if dist <= radius*radius {
				pixels[idx+0] = b // B
				pixels[idx+1] = g // G
				pixels[idx+2] = r // R
				pixels[idx+3] = 0xFF // A
			} else {
				pixels[idx+0] = 0 // B
				pixels[idx+1] = 0 // G
				pixels[idx+2] = 0 // R
				pixels[idx+3] = 0 // A (transparent)
			}
		}
	}

	// AND mask (1bpp, all zeros = fully visible)
	andMask := make([]byte, size*size/8)

	// BITMAPINFOHEADER (40 bytes)
	bih := make([]byte, 40)
	le32(bih[0:], 40)             // biSize
	le32(bih[4:], uint32(size))   // biWidth
	le32(bih[8:], uint32(size*2)) // biHeight (doubled for ICO)
	le16(bih[12:], 1)             // biPlanes
	le16(bih[14:], 32)            // biBitCount
	// rest is zeros (no compression, etc.)

	imageData := append(bih, pixels...)
	imageData = append(imageData, andMask...)

	// ICO header
	ico := make([]byte, 6+16) // ICONDIR + 1 ICONDIRENTRY
	le16(ico[0:], 0)          // reserved
	le16(ico[2:], 1)          // type: icon
	le16(ico[4:], 1)          // count: 1 image

	// ICONDIRENTRY
	ico[6] = byte(size) // width
	ico[7] = byte(size) // height
	ico[8] = 0          // colors (0=no palette)
	ico[9] = 0          // reserved
	le16(ico[10:], 1)   // planes
	le16(ico[12:], 32)  // bit count
	le32(ico[14:], uint32(len(imageData)))
	le32(ico[18:], uint32(len(ico))) // offset

	return append(ico, imageData...)
}

func le16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func le32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

package main

// Embedded system tray icons as ICO-format byte arrays.

// iconConnected is a green shield icon (connected state)
var iconConnected = generateShieldIcon(0x00, 0xBB, 0x55, true) // green, checkmark

// iconDisconnected is a gray shield icon (disconnected state)
var iconDisconnected = generateShieldIcon(0x88, 0x88, 0x88, false) // gray, no mark

// iconConnecting is a yellow shield icon (connecting state)
var iconConnecting = generateShieldIcon(0xDD, 0xAA, 0x00, false) // yellow, no mark

// generateShieldIcon creates a 16x16 ICO with a shield shape and optional checkmark.
func generateShieldIcon(r, g, b byte, checkmark bool) []byte {
	const size = 16

	pixels := make([]byte, size*size*4)

	// Shield shape mask: a rounded shield outline
	shield := [16][16]bool{
		//  0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15
		{F, F, F, T, T, T, T, T, T, T, T, T, T, F, F, F}, // 0
		{F, F, T, T, T, T, T, T, T, T, T, T, T, T, F, F}, // 1
		{F, T, T, T, T, T, T, T, T, T, T, T, T, T, T, F}, // 2
		{F, T, T, T, T, T, T, T, T, T, T, T, T, T, T, F}, // 3
		{F, T, T, T, T, T, T, T, T, T, T, T, T, T, T, F}, // 4
		{F, T, T, T, T, T, T, T, T, T, T, T, T, T, T, F}, // 5
		{F, T, T, T, T, T, T, T, T, T, T, T, T, T, T, F}, // 6
		{F, T, T, T, T, T, T, T, T, T, T, T, T, T, T, F}, // 7
		{F, T, T, T, T, T, T, T, T, T, T, T, T, T, T, F}, // 8
		{F, F, T, T, T, T, T, T, T, T, T, T, T, T, F, F}, // 9
		{F, F, T, T, T, T, T, T, T, T, T, T, T, T, F, F}, // 10
		{F, F, F, T, T, T, T, T, T, T, T, T, T, F, F, F}, // 11
		{F, F, F, F, T, T, T, T, T, T, T, T, F, F, F, F}, // 12
		{F, F, F, F, F, T, T, T, T, T, T, F, F, F, F, F}, // 13
		{F, F, F, F, F, F, T, T, T, T, F, F, F, F, F, F}, // 14
		{F, F, F, F, F, F, F, T, T, F, F, F, F, F, F, F}, // 15
	}

	// Checkmark pixels (drawn inside the shield when connected)
	check := [16][16]bool{}
	if checkmark {
		// Simple checkmark shape
		check[5][10] = true; check[5][11] = true
		check[6][9] = true; check[6][10] = true; check[6][11] = true
		check[7][8] = true; check[7][9] = true; check[7][10] = true
		check[8][5] = true; check[8][7] = true; check[8][8] = true; check[8][9] = true
		check[9][5] = true; check[9][6] = true; check[9][7] = true; check[9][8] = true
		check[10][6] = true; check[10][7] = true
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			row := size - 1 - y // bottom-up BMP
			idx := (row*size + x) * 4
			if shield[y][x] {
				if checkmark && check[y][x] {
					// White checkmark
					pixels[idx+0] = 0xFF // B
					pixels[idx+1] = 0xFF // G
					pixels[idx+2] = 0xFF // R
					pixels[idx+3] = 0xFF // A
				} else {
					pixels[idx+0] = b    // B
					pixels[idx+1] = g    // G
					pixels[idx+2] = r    // R
					pixels[idx+3] = 0xFF // A
				}
			}
			// else: transparent (all zeros)
		}
	}

	andMask := make([]byte, size*size/8)

	bih := make([]byte, 40)
	le32(bih[0:], 40)
	le32(bih[4:], uint32(size))
	le32(bih[8:], uint32(size*2))
	le16(bih[12:], 1)
	le16(bih[14:], 32)

	imageData := append(bih, pixels...)
	imageData = append(imageData, andMask...)

	ico := make([]byte, 6+16)
	le16(ico[0:], 0)
	le16(ico[2:], 1)
	le16(ico[4:], 1)
	ico[6] = byte(size)
	ico[7] = byte(size)
	ico[8] = 0
	ico[9] = 0
	le16(ico[10:], 1)
	le16(ico[12:], 32)
	le32(ico[14:], uint32(len(imageData)))
	le32(ico[18:], uint32(len(ico)))

	return append(ico, imageData...)
}

const (
	T = true
	F = false
)

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

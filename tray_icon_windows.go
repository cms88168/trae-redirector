//go:build windows
// +build windows

package main

// defaultTrayIcon 是内嵌的 32x32 8bpp ICO 图标
// 设计：蓝色圆角背景上的白色闪电/箭头符号，代表重定向
// 使用纯Go字节数组，无需外部文件依赖
var defaultTrayIcon []byte

func init() {
	// 生成一个32x32 32bpp(BGRA)的现代风格图标
	// 蓝色渐变圆形背景 + 白色右箭头 "→" 符号
	const (
		width      = 32
		height     = 32
		bpp        = 32
		imageSize  = width * height * 4 // BGRA每像素4字节
		headerSize = 40                 // BITMAPINFOHEADER
		dataOffset = 6 + 16 + headerSize
		fileSize   = dataOffset + imageSize + (width * height / 8) // + AND mask
		andSize    = width * height / 8
	)

	icon := make([]byte, 0, fileSize)

	// ICONDIR (6 bytes)
	icon = append(icon, 0x00, 0x00) // reserved
	icon = append(icon, 0x01, 0x00) // type = ICO
	icon = append(icon, 0x01, 0x00) // count = 1

	// ICONDIRENTRY (16 bytes)
	totalDataSize := uint32(headerSize + imageSize + andSize)
	icon = append(icon, byte(width), byte(height)) // width, height
	icon = append(icon, 0x00)                      // color count (0 = >256)
	icon = append(icon, 0x00)                      // reserved
	icon = append(icon, 0x01, 0x00)                // planes
	icon = append(icon, byte(bpp), 0x00)           // bit count
	// bytes in resource (little-endian uint32)
	icon = append(icon, byte(totalDataSize), byte(totalDataSize>>8), byte(totalDataSize>>16), byte(totalDataSize>>24))
	// offset to data
	offset := uint32(6 + 16)
	icon = append(icon, byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24))

	// BITMAPINFOHEADER (40 bytes)
	icon = append(icon, 40, 0, 0, 0) // biSize
	icon = append(icon, byte(width), 0, 0, 0) // biWidth
	icon = append(icon, byte(height*2), 0, 0, 0) // biHeight (doubled for AND mask)
	icon = append(icon, 1, 0)          // biPlanes
	icon = append(icon, byte(bpp), 0)  // biBitCount
	icon = append(icon, 0, 0, 0, 0)   // biCompression
	// biSizeImage (little-endian uint32)
	imgSz := uint32(imageSize)
	icon = append(icon, byte(imgSz), byte(imgSz>>8), byte(imgSz>>16), byte(imgSz>>24))
	icon = append(icon, 0, 0, 0, 0)   // biXPelsPerMeter
	icon = append(icon, 0, 0, 0, 0)   // biYPelsPerMeter
	icon = append(icon, 0, 0, 0, 0)   // biClrUsed
	icon = append(icon, 0, 0, 0, 0)   // biClrImportant

	// Pixel data (BGRA, bottom-up)
	pixels := make([]byte, imageSize)
	for row := 0; row < height; row++ {
		y := height - 1 - row // BMP is bottom-up
		for x := 0; x < width; x++ {
			idx := (row*width + x) * 4
			// 计算到中心的距离
			cx := float64(x) - 15.5
			cy := float64(y) - 15.5
			dist := cx*cx + cy*cy

			if dist < 14.5*14.5 {
				// 圆形内部：渐变蓝色背景
				// 从深蓝(#1a73e8)到浅蓝(#4fc3f7)的径向渐变
				t := dist / (14.5 * 14.5) // 0~1
				b := uint8(0xe8 - uint8(t*80))
				g := uint8(0x73 + uint8(t*60))
				r := uint8(0x1a + uint8(t*40))

				// 绘制白色箭头 "→" 符号
				isArrow := false
				// 箭头主体（水平线）: y=15~16, x=8~22
				if y >= 14 && y <= 17 && x >= 9 && x <= 22 {
					isArrow = true
				}
				// 箭头上半三角: 从(22,15)到(18,11)
				if x >= 18 && x <= 23 {
					dy := 15 - y // 向上偏移
					dx := x - 18
					if dy >= 0 && dy <= 5 && dx >= dy-1 && dx <= dy+1 {
						isArrow = true
					}
				}
				// 箭头下半三角: 从(22,16)到(18,20)
				if x >= 18 && x <= 23 {
					dy := y - 16 // 向下偏移
					dx := x - 18
					if dy >= 0 && dy <= 5 && dx >= dy-1 && dx <= dy+1 {
						isArrow = true
					}
				}

				if isArrow {
					// 白色箭头
					pixels[idx+0] = 0xFF // B
					pixels[idx+1] = 0xFF // G
					pixels[idx+2] = 0xFF // R
					pixels[idx+3] = 0xFF // A
				} else {
					// 蓝色背景
					pixels[idx+0] = b    // B
					pixels[idx+1] = g    // G
					pixels[idx+2] = r    // R
					pixels[idx+3] = 0xFF // A
				}
			} else if dist < 15.5*15.5 {
				// 边缘抗锯齿：半透明
				pixels[idx+0] = 0xA0 // B
				pixels[idx+1] = 0x60 // G
				pixels[idx+2] = 0x15 // R
				pixels[idx+3] = 0x80 // A (半透明)
			} else {
				// 圆形外部：全透明
				pixels[idx+0] = 0x00
				pixels[idx+1] = 0x00
				pixels[idx+2] = 0x00
				pixels[idx+3] = 0x00
			}
		}
	}
	icon = append(icon, pixels...)

	// AND mask (全0 = 完全不透明，由alpha通道控制透明度)
	andMask := make([]byte, andSize)
	icon = append(icon, andMask...)

	defaultTrayIcon = icon
}

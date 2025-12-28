package ApiUtils

import (
	"fmt"
	"os/exec"
)

func PdfPageToImageWithMagick(
		pdfPath string, 
		density int,
		pageNumber int, 
		imagePath string) error {
    // ImageMagick command: magick -density <density> "input.pdf[pageNumber]" -colorspace sRGB -alpha remove -alpha off <imagePath>
	command := fmt.Sprintf("magick -dentity %d \"%s[%d]\" -colorspace sRGB -alpha remove -alpha off %s",
			density, pdfPath, pageNumber, imagePath)
	cmd := exec.Command(command)
    // cmd := exec.Command("convert", 
    //     "-density", "300", 
    //     fmt.Sprintf("%s[%d]", pdfPath, pageNumber-1), // 0-based indexing
    //     imagePath)
    
    err := cmd.Run()
    if err != nil {
        return fmt.Errorf("failed to convert PDF to image: %v", err)
    }
    
    return nil
}
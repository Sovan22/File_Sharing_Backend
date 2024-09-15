package utils

import(
	"strings"
)

func ExtractType(filename string) string {
    lastDotIndex := strings.LastIndex(filename, ".")
    if lastDotIndex == -1 {
        return "empty" // No dot found, return empty
    }
    return filename[lastDotIndex+1:]
}
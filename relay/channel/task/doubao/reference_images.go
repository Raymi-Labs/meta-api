package doubao

import (
	"fmt"
	"net/url"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Resolve capabilities against the mapped upstream ID, never the public alias.
func normalizeSeedanceImages(req *relaycommon.TaskSubmitReq, body *requestPayload) error {
	if body.Model != "doubao-seedance-2-5-260628" && body.Model != "dreamina-seedance-2-5-260628" {
		return nil
	}
	mode := req.Mode
	switch mode {
	case "", "reference", "first_frame", "first_last_frame", "text_to_video":
	default:
		return fmt.Errorf("unsupported Seedance photo mode: %s", mode)
	}
	images := req.Images
	if req.Image != "" && len(images) > 0 && (len(images) != 1 || images[0] != req.Image) {
		return fmt.Errorf("use either image or images")
	}
	if req.InputReference != "" {
		if len(images) > 0 {
			return fmt.Errorf("use either images/image or input_reference")
		}
		images = []string{req.InputReference}
	}
	if _, exists := req.Metadata["content"]; exists && len(images) > 0 {
		return fmt.Errorf("use either images/input_reference or metadata.content")
	}
	if len(images) > 0 {
		if mode == "" {
			mode = "reference"
		}
		body.Content = nil
		if req.Prompt != "" {
			body.Content = append(body.Content, ContentItem{Type: "text", Text: req.Prompt})
		}
		for i, image := range images {
			role := "reference_image"
			if mode == "first_frame" || mode == "first_last_frame" {
				role = "first_frame"
				if i > 0 {
					role = "last_frame"
				}
			}
			body.Content = append(body.Content, ContentItem{Type: "image_url", ImageURL: &ImageURL{URL: image}, Role: role})
		}
	}
	first, last, refs := 0, 0, 0
	for i := range body.Content {
		item := &body.Content[i]
		if item.Type == "video_url" || item.Type == "audio_url" {
			return fmt.Errorf("Seedance reference video/audio inputs are not implemented by this photo adapter")
		}
		if item.Type != "image_url" {
			continue
		}
		if item.ImageURL == nil || !validPhotoURL(item.ImageURL.URL) {
			return fmt.Errorf("invalid Seedance photo URL")
		}
		if item.Role == "" {
			if mode == "first_frame" || mode == "first_last_frame" {
				return fmt.Errorf("metadata.content frame images require explicit roles")
			}
			item.Role = "reference_image"
		}
		switch item.Role {
		case "first_frame":
			first++
		case "last_frame":
			last++
		case "reference_image":
			refs++
		default:
			return fmt.Errorf("invalid Seedance image role: %s", item.Role)
		}
	}
	if refs > 30 || first > 1 || last > 1 || (last > 0 && first == 0) {
		return fmt.Errorf("Seedance supports up to 30 reference photos or one first/last frame pair")
	}
	if refs > 0 && first+last > 0 {
		return fmt.Errorf("reference photos cannot be mixed with first/last frames")
	}
	switch mode {
	case "reference":
		if refs == 0 || first+last > 0 {
			return fmt.Errorf("reference mode requires reference photos only")
		}
	case "first_frame":
		if first != 1 || last+refs != 0 {
			return fmt.Errorf("first_frame mode requires exactly one first frame")
		}
	case "first_last_frame":
		if first != 1 || last != 1 || refs != 0 {
			return fmt.Errorf("first_last_frame mode requires a first and last frame")
		}
	case "text_to_video":
		if first+last+refs > 0 {
			return fmt.Errorf("text_to_video mode cannot include photos")
		}
	}
	if first+last > 0 {
		if body.Ratio != "" && body.Ratio != "adaptive" {
			return fmt.Errorf("Seedance frame mode requires ratio=adaptive")
		}
		body.Ratio = "adaptive"
	}
	return nil
}

func validPhotoURL(value string) bool {
	if value != strings.TrimSpace(value) || value == "" {
		return false
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "https", "http":
		return u.Host != "" && u.User == nil
	case "asset":
		return u.Host != ""
	case "data":
		for _, mime := range []string{"image/png", "image/jpeg", "image/webp"} {
			prefix := "data:" + mime + ";base64,"
			if strings.HasPrefix(value, prefix) {
				return len(value) > len(prefix)
			}
		}
	}
	return false
}

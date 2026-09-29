package packs

// Category describes one downloadable index pack.
type Category struct {
	ID             string
	Title          string
	Pipeline       string
	Filter         string
	ExtraPipelines []string
}

// DefaultCategories matches runhug v1 packs.
func DefaultCategories() []Category {
	return []Category{
		{ID: "text-generation", Title: "Text Generation (LLMs)", Pipeline: "text-generation"},
		{ID: "text-to-image", Title: "Text to Image", Pipeline: "text-to-image"},
		{ID: "video", Title: "Video (text/image → video)", Pipeline: "text-to-video", ExtraPipelines: []string{"image-to-video"}},
		{ID: "audio", Title: "Audio (TTS / ASR)", Pipeline: "text-to-speech", ExtraPipelines: []string{"automatic-speech-recognition"}},
		{ID: "gguf", Title: "GGUF (local weights)", Filter: "gguf"},
	}
}

// LookupCategory returns a category by id.
func LookupCategory(id string) (Category, bool) {
	for _, c := range DefaultCategories() {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}

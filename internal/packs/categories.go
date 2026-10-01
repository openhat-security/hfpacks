package packs

import "strings"

// Category describes one downloadable index pack / search type.
type Category struct {
	ID             string
	Title          string
	Pipeline       string
	Filter         string
	ExtraPipelines []string
}

// TypeSpec is a broad --type bucket or Hub pipeline alias.
type TypeSpec struct {
	ID          string
	Title       string
	Pipelines   []string
	Filter      string
	Aliases     []string
	ReleasePack bool
}

func catalog() []TypeSpec {
	return []TypeSpec{
		{
			ID: "text-generation", Title: "Text Generation (LLMs)",
			Pipelines:   []string{"text-generation"},
			Aliases:     []string{"llm", "llms", "text-to-text", "chat"},
			ReleasePack: true,
		},
		{
			ID: "vision", Title: "Vision / Multimodal",
			Pipelines: []string{
				"image-text-to-text", "visual-question-answering",
				"image-to-text", "any-to-any", "document-question-answering",
			},
			Aliases:     []string{"multimodal", "vlm", "vision-language"},
			ReleasePack: true,
		},
		{
			ID: "text-to-image", Title: "Text to Image",
			Pipelines:   []string{"text-to-image", "image-to-image"},
			Aliases:     []string{"image", "diffusion", "img"},
			ReleasePack: true,
		},
		{
			ID: "video", Title: "Video (text/image → video)",
			Pipelines:   []string{"text-to-video", "image-to-video"},
			Aliases:     []string{"text-to-video"},
			ReleasePack: true,
		},
		{
			ID: "audio", Title: "Audio (TTS / ASR / music)",
			Pipelines: []string{
				"text-to-speech", "automatic-speech-recognition", "text-to-audio",
			},
			Aliases:     []string{"speech", "tts", "asr"},
			ReleasePack: true,
		},
		{
			ID: "embeddings", Title: "Embeddings / feature extraction",
			Pipelines:   []string{"feature-extraction", "sentence-similarity"},
			Aliases:     []string{"embedding", "feature-extraction", "sentence-similarity"},
			ReleasePack: true,
		},
		{
			ID: "gguf", Title: "GGUF (local weights)",
			Filter: "gguf", Aliases: []string{"llama-cpp", "llamacpp"},
			ReleasePack: true,
		},
		hubType("fill-mask", "Fill-Mask"),
		hubType("token-classification", "Token Classification"),
		hubType("text-classification", "Text Classification"),
		hubType("question-answering", "Question Answering"),
		hubType("summarization", "Summarization"),
		hubType("translation", "Translation"),
		hubType("text2text-generation", "Text2Text Generation"),
		hubType("reinforcement-learning", "Reinforcement Learning"),
		hubType("object-detection", "Object Detection"),
		hubType("image-classification", "Image Classification"),
		hubType("image-segmentation", "Image Segmentation"),
	}
}

func hubType(id, title string) TypeSpec {
	return TypeSpec{ID: id, Title: title, Pipelines: []string{id}}
}

func DefaultCategories() []Category {
	var out []Category
	for _, t := range catalog() {
		if t.ReleasePack {
			out = append(out, typeToCategory(t))
		}
	}
	return out
}

func typeToCategory(t TypeSpec) Category {
	c := Category{ID: t.ID, Title: t.Title, Filter: t.Filter}
	if len(t.Pipelines) > 0 {
		c.Pipeline = t.Pipelines[0]
		if len(t.Pipelines) > 1 {
			c.ExtraPipelines = append([]string{}, t.Pipelines[1:]...)
		}
	}
	return c
}

func LookupType(id string) (TypeSpec, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return TypeSpec{}, false
	}
	for _, t := range catalog() {
		if strings.EqualFold(t.ID, key) {
			return t, true
		}
		for _, a := range t.Aliases {
			if strings.EqualFold(a, key) {
				return t, true
			}
		}
	}
	return TypeSpec{}, false
}

func ExpandType(id string) (pipelines []string, filter string, ok bool) {
	t, ok := LookupType(id)
	if !ok {
		return nil, "", false
	}
	return append([]string{}, t.Pipelines...), t.Filter, true
}

func LookupCategory(id string) (Category, bool) {
	t, ok := LookupType(id)
	if !ok || !t.ReleasePack {
		return Category{}, false
	}
	return typeToCategory(t), true
}

func CategoryIDs() []string {
	cats := DefaultCategories()
	out := make([]string, len(cats))
	for i, c := range cats {
		out[i] = c.ID
	}
	return out
}

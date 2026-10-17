package classify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"

	"github.com/arbhalerao/picseek/internal/store"
)

const (
	clipScale = 100.0
	textScale = 10.0
	scheme    = "best-label-with-probability"
)

type Label struct {
	Name    string
	Visual  []string
	Textual []string
}

const none = ""

var Labels = []Label{
	{"money", []string{"a screenshot of a receipt", "a screenshot of an invoice or a bill", "a screenshot of a bank transaction", "a screenshot of a payment app"},
		[]string{"receipt with items, tax and total amount paid", "invoice or bill with the amount due", "bank account debited or credited with the transaction amount", "payment successful or payment failed"}},
	{"travel", []string{"a screenshot of a boarding pass", "a screenshot of a flight or train ticket", "a screenshot of a travel booking"},
		[]string{"boarding pass with flight number, gate and seat", "train or bus ticket with departure time and PNR", "hotel or flight booking confirmation"}},
	{"message", []string{"a screenshot of a chat conversation", "a screenshot of a messaging app"},
		[]string{"chat messages between people with replies", "conversation in a messaging app"}},
	{"dev", []string{"a screenshot of source code in an editor", "a screenshot of a terminal window", "a screenshot of a stack trace"},
		[]string{"source code with functions, imports and variables", "terminal commands and their output", "error traceback with an exception and a line number"}},
	{"security", []string{"a screenshot of a text message with a verification code", "a screenshot of a sign-in screen"},
		[]string{"your one-time password verification code is", "do not share this code with anyone", "sign in to your account with your password"}},
	{"document", []string{"a screenshot of a document page", "a screenshot of a pdf", "a screenshot of an article", "a screenshot of a note"},
		[]string{"document page with headings and paragraphs of text", "article or report with long paragraphs", "a note or a list"}},
	{"chart", []string{"a screenshot of a bar chart", "a screenshot of a line graph", "a screenshot of a pie chart", "a screenshot of a dashboard with charts"},
		[]string{"chart of values by month with axis labels", "dashboard with metrics and percentages"}},
	{none, []string{"a screenshot of a settings screen", "a screenshot of a dialog box", "a screenshot of a form", "a screenshot of an app's home screen"},
		[]string{"settings options that can be turned on or off", "a message box with a button"}},
}

type Embedder interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, string, error)
	EmbedClipTexts(ctx context.Context, texts []string) ([][]float32, string, error)
}

type Classifier struct {
	visual  [][][]float32
	textual [][][]float32
	Version string
}

func New(ctx context.Context, e Embedder) (*Classifier, error) {
	c := &Classifier{visual: make([][][]float32, len(Labels)), textual: make([][][]float32, len(Labels))}
	h := sha256.New()
	h.Write([]byte(scheme))
	for i, l := range Labels {
		vectors, clipVersion, err := e.EmbedClipTexts(ctx, l.Visual)
		if err != nil {
			return nil, err
		}
		c.visual[i] = vectors
		h.Write([]byte(l.Name + "|" + strings.Join(l.Visual, "|") + "|" + clipVersion))
		if len(l.Textual) > 0 {
			vectors, textVersion, err := e.EmbedTexts(ctx, l.Textual)
			if err != nil {
				return nil, err
			}
			c.textual[i] = vectors
			h.Write([]byte(strings.Join(l.Textual, "|") + "|" + textVersion))
		}
	}
	c.Version = "zero-shot-" + hex.EncodeToString(h.Sum(nil))[:12]
	return c, nil
}

func (c *Classifier) Classify(in store.ClassifyInput) store.ImageType {
	visual := make([]float64, len(Labels))
	for i := range Labels {
		visual[i] = clipScale * best(in.Image, c.visual[i])
	}
	probs := softmax(visual)

	if len(in.Text) > 0 {
		textual := make([]float64, len(Labels))
		for i := range Labels {
			score := 0.0
			for _, chunk := range in.Text {
				score = math.Max(score, best(chunk, c.textual[i]))
			}
			textual[i] = textScale * score
		}
		for i, p := range softmax(textual) {
			probs[i] = (probs[i] + p) / 2
		}
	}

	top := 0
	for i := range probs {
		if probs[i] > probs[top] {
			top = i
		}
	}
	return store.ImageType{ImageID: in.ImageID, Type: Labels[top].Name, Score: probs[top]}
}

func best(v []float32, prompts [][]float32) float64 {
	score := math.Inf(-1)
	for _, p := range prompts {
		score = math.Max(score, dot(v, p))
	}
	if math.IsInf(score, -1) {
		return 0
	}
	return score
}

func dot(a, b []float32) float64 {
	var sum float32
	for i := range min(len(a), len(b)) {
		sum += a[i] * b[i]
	}
	return float64(sum)
}

func softmax(x []float64) []float64 {
	maxX := math.Inf(-1)
	for _, v := range x {
		maxX = math.Max(maxX, v)
	}
	out := make([]float64, len(x))
	var sum float64
	for i, v := range x {
		out[i] = math.Exp(v - maxX)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

//go:build onnx

package embeddings

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

// Tokenizer converts text into ONNX-compatible token sequences. Both the
// HuggingFace fast tokenizer (tokenizer.json) and the vocab.txt WordPiece
// fallback satisfy this interface, so the ONNX embedder can use whichever
// the model directory provides.
type Tokenizer interface {
	Tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64)
}

// WordPieceTokenizer is the vocab.txt-based fallback used when tokenizer.json
// is unavailable. It mirrors BERT's classic WordPiece algorithm.
type WordPieceTokenizer struct {
	vocab  map[string]int32
	unkID  int32
	clsID  int32
	sepID  int32
	padID  int32
	maxLen int
}

func NewWordPieceTokenizer(vocabPath string, maxLen int) (*WordPieceTokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int32)
	scanner := bufio.NewScanner(f)
	var id int32
	for scanner.Scan() {
		token := scanner.Text()
		vocab[token] = id
		id++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if maxLen <= 0 {
		maxLen = 128
	}

	return &WordPieceTokenizer{
		vocab:  vocab,
		unkID:  vocab["[UNK]"],
		clsID:  vocab["[CLS]"],
		sepID:  vocab["[SEP]"],
		padID:  vocab["[PAD]"],
		maxLen: maxLen,
	}, nil
}

func (t *WordPieceTokenizer) Tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64) {
	text = strings.ToLower(strings.TrimSpace(text))

	words := SplitOnPunctuation(text)

	var tokens []int32
	tokens = append(tokens, t.clsID)
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		pieces := t.wordPieceEncode(word)
		tokens = append(tokens, pieces...)
		if len(tokens) >= t.maxLen-1 {
			tokens = tokens[:t.maxLen-1]
			break
		}
	}
	tokens = append(tokens, t.sepID)

	seqLen := len(tokens)

	inputIDs = make([]int64, t.maxLen)
	attentionMask = make([]int64, t.maxLen)
	tokenTypeIDs = make([]int64, t.maxLen)

	for i := 0; i < seqLen; i++ {
		inputIDs[i] = int64(tokens[i])
		attentionMask[i] = 1
	}
	for i := seqLen; i < t.maxLen; i++ {
		inputIDs[i] = int64(t.padID)
	}

	return inputIDs, attentionMask, tokenTypeIDs
}

func (t *WordPieceTokenizer) wordPieceEncode(word string) []int32 {
	if _, ok := t.vocab[word]; ok {
		return []int32{t.vocab[word]}
	}

	var tokens []int32
	start := 0
	for start < len(word) {
		end := len(word)
		found := false
		for end > start {
			sub := word[start:end]
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := t.vocab[sub]; ok {
				tokens = append(tokens, id)
				found = true
				start = end
				break
			}
			end--
		}
		if !found {
			tokens = append(tokens, t.unkID)
			start++
		}
	}
	return tokens
}

// SplitOnPunctuation splits text into words, isolating punctuation and symbol
// runes as their own tokens. Shared by both tokenizer implementations.
func SplitOnPunctuation(text string) []string {
	var words []string
	var current strings.Builder
	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, string(r))
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

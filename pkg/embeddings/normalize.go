// Package embeddings provides MLX embedding generation for semantic search.
package embeddings

import (
	"math"
)

// Normalize normalizes a vector to unit length (L2 normalization).
// This is required for cosine similarity calculations when using dot product.
// Returns a new normalized vector; the input is not modified.
// Returns the zero vector if the input has zero magnitude.
func Normalize(vector []float32) []float32 {
	if len(vector) == 0 {
		return vector
	}

	magnitude := Magnitude(vector)
	if magnitude == 0 {
		// Return zero vector if magnitude is zero
		return make([]float32, len(vector))
	}

	result := make([]float32, len(vector))
	for i, v := range vector {
		result[i] = v / magnitude
	}

	return result
}

// NormalizeInPlace normalizes a vector in place (modifies the input).
// Returns the magnitude of the original vector.
func NormalizeInPlace(vector []float32) float32 {
	if len(vector) == 0 {
		return 0
	}

	magnitude := Magnitude(vector)
	if magnitude == 0 {
		return 0
	}

	for i := range vector {
		vector[i] /= magnitude
	}

	return magnitude
}

// Magnitude returns the L2 (Euclidean) magnitude of a vector.
func Magnitude(vector []float32) float32 {
	if len(vector) == 0 {
		return 0
	}

	var sum float64
	for _, v := range vector {
		sum += float64(v) * float64(v)
	}

	return float32(math.Sqrt(sum))
}

// CosineSimilarity calculates the cosine similarity between two vectors.
// The result ranges from -1 (opposite) to 1 (identical), with 0 being orthogonal.
// For normalized vectors, this is equivalent to the dot product.
//
// Returns 0 if either vector has zero magnitude or if dimensions don't match.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct float64
	var magA, magB float64

	for i := 0; i < len(a); i++ {
		aVal := float64(a[i])
		bVal := float64(b[i])
		dotProduct += aVal * bVal
		magA += aVal * aVal
		magB += bVal * bVal
	}

	magA = math.Sqrt(magA)
	magB = math.Sqrt(magB)

	if magA == 0 || magB == 0 {
		return 0
	}

	return float32(dotProduct / (magA * magB))
}

// DotProduct calculates the dot product of two vectors.
// For normalized vectors, this equals cosine similarity.
// Returns 0 if dimensions don't match.
func DotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		sum += float64(a[i]) * float64(b[i])
	}

	return float32(sum)
}

// EuclideanDistance calculates the Euclidean (L2) distance between two vectors.
// Returns -1 if dimensions don't match.
func EuclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return -1
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		diff := float64(a[i]) - float64(b[i])
		sum += diff * diff
	}

	return float32(math.Sqrt(sum))
}

// ManhattanDistance calculates the Manhattan (L1) distance between two vectors.
// Returns -1 if dimensions don't match.
func ManhattanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return -1
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		sum += math.Abs(float64(a[i]) - float64(b[i]))
	}

	return float32(sum)
}

// CosineDistance calculates the cosine distance between two vectors.
// This is 1 - CosineSimilarity, ranging from 0 (identical) to 2 (opposite).
func CosineDistance(a, b []float32) float32 {
	return 1 - CosineSimilarity(a, b)
}

// Add adds two vectors element-wise.
// Returns nil if dimensions don't match.
func Add(a, b []float32) []float32 {
	if len(a) != len(b) {
		return nil
	}

	result := make([]float32, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = a[i] + b[i]
	}

	return result
}

// Subtract subtracts vector b from vector a element-wise.
// Returns nil if dimensions don't match.
func Subtract(a, b []float32) []float32 {
	if len(a) != len(b) {
		return nil
	}

	result := make([]float32, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = a[i] - b[i]
	}

	return result
}

// Scale multiplies a vector by a scalar.
func Scale(vector []float32, scalar float32) []float32 {
	result := make([]float32, len(vector))
	for i, v := range vector {
		result[i] = v * scalar
	}
	return result
}

// Mean calculates the element-wise mean of multiple vectors.
// Returns nil if no vectors are provided or dimensions don't match.
func Mean(vectors [][]float32) []float32 {
	if len(vectors) == 0 {
		return nil
	}

	dims := len(vectors[0])
	if dims == 0 {
		return nil
	}

	// Check all vectors have same dimensions
	for _, v := range vectors[1:] {
		if len(v) != dims {
			return nil
		}
	}

	result := make([]float32, dims)
	for _, vector := range vectors {
		for i, v := range vector {
			result[i] += v
		}
	}

	n := float32(len(vectors))
	for i := range result {
		result[i] /= n
	}

	return result
}

// TopK finds the k vectors most similar to a query vector.
// Returns indices and similarity scores, sorted by similarity (descending).
type SimilarityResult struct {
	Index      int
	Similarity float32
}

// FindTopK finds the k most similar vectors to the query.
// Returns results sorted by similarity in descending order.
func FindTopK(query []float32, vectors [][]float32, k int) []SimilarityResult {
	if k <= 0 || len(vectors) == 0 {
		return nil
	}

	if k > len(vectors) {
		k = len(vectors)
	}

	// Calculate all similarities
	similarities := make([]SimilarityResult, len(vectors))
	for i, v := range vectors {
		similarities[i] = SimilarityResult{
			Index:      i,
			Similarity: CosineSimilarity(query, v),
		}
	}

	// Sort by similarity (descending) using partial sort for efficiency
	// For small k, this is more efficient than full sort
	for i := 0; i < k; i++ {
		maxIdx := i
		for j := i + 1; j < len(similarities); j++ {
			if similarities[j].Similarity > similarities[maxIdx].Similarity {
				maxIdx = j
			}
		}
		if maxIdx != i {
			similarities[i], similarities[maxIdx] = similarities[maxIdx], similarities[i]
		}
	}

	return similarities[:k]
}

// IsNormalized checks if a vector is approximately normalized (unit length).
// Uses a small tolerance for floating-point comparison.
func IsNormalized(vector []float32) bool {
	if len(vector) == 0 {
		return false
	}

	mag := Magnitude(vector)
	const tolerance = 1e-6
	return math.Abs(float64(mag)-1.0) < tolerance
}

// ZeroVector creates a zero vector of the specified dimension.
func ZeroVector(dims int) []float32 {
	return make([]float32, dims)
}

// RandomVector creates a random unit vector of the specified dimension.
// This is useful for testing purposes.
// Note: This is a simple implementation; not cryptographically secure.
func RandomVector(dims int, seed int64) []float32 {
	// Simple LCG random number generator
	m := int64(1 << 31)
	a := int64(1103515245)
	c := int64(12345)
	state := seed

	vector := make([]float32, dims)
	for i := 0; i < dims; i++ {
		state = (a*state + c) % m
		// Convert to float in range [-1, 1]
		vector[i] = float32(state)/float32(m)*2 - 1
	}

	// Normalize to unit length
	return Normalize(vector)
}

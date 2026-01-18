package embeddings

import (
	"math"
	"testing"
)

const tolerance = 1e-6

func floatEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < tolerance
}

func TestMagnitude(t *testing.T) {
	t.Run("empty vector", func(t *testing.T) {
		result := Magnitude([]float32{})
		if result != 0 {
			t.Errorf("Magnitude([]) = %f, want 0", result)
		}
	})

	t.Run("zero vector", func(t *testing.T) {
		result := Magnitude([]float32{0, 0, 0})
		if result != 0 {
			t.Errorf("Magnitude([0,0,0]) = %f, want 0", result)
		}
	})

	t.Run("unit vector", func(t *testing.T) {
		result := Magnitude([]float32{1, 0, 0})
		if !floatEqual(result, 1) {
			t.Errorf("Magnitude([1,0,0]) = %f, want 1", result)
		}
	})

	t.Run("3-4-5 triangle", func(t *testing.T) {
		result := Magnitude([]float32{3, 4, 0})
		if !floatEqual(result, 5) {
			t.Errorf("Magnitude([3,4,0]) = %f, want 5", result)
		}
	})

	t.Run("general vector", func(t *testing.T) {
		// [1, 2, 2] -> sqrt(1 + 4 + 4) = sqrt(9) = 3
		result := Magnitude([]float32{1, 2, 2})
		if !floatEqual(result, 3) {
			t.Errorf("Magnitude([1,2,2]) = %f, want 3", result)
		}
	})
}

func TestNormalize(t *testing.T) {
	t.Run("empty vector", func(t *testing.T) {
		result := Normalize([]float32{})
		if len(result) != 0 {
			t.Errorf("Normalize([]) length = %d, want 0", len(result))
		}
	})

	t.Run("zero vector", func(t *testing.T) {
		result := Normalize([]float32{0, 0, 0})
		for i, v := range result {
			if v != 0 {
				t.Errorf("Normalize([0,0,0])[%d] = %f, want 0", i, v)
			}
		}
	})

	t.Run("already normalized", func(t *testing.T) {
		input := []float32{1, 0, 0}
		result := Normalize(input)
		if !floatEqual(result[0], 1) || result[1] != 0 || result[2] != 0 {
			t.Errorf("Normalize([1,0,0]) = %v, want [1,0,0]", result)
		}
	})

	t.Run("general vector", func(t *testing.T) {
		result := Normalize([]float32{3, 4, 0})
		// Magnitude is 5, so normalized is [0.6, 0.8, 0]
		if !floatEqual(result[0], 0.6) {
			t.Errorf("result[0] = %f, want 0.6", result[0])
		}
		if !floatEqual(result[1], 0.8) {
			t.Errorf("result[1] = %f, want 0.8", result[1])
		}
		if result[2] != 0 {
			t.Errorf("result[2] = %f, want 0", result[2])
		}

		// Verify result is normalized
		mag := Magnitude(result)
		if !floatEqual(mag, 1) {
			t.Errorf("Magnitude of normalized vector = %f, want 1", mag)
		}
	})

	t.Run("does not modify input", func(t *testing.T) {
		input := []float32{3, 4, 0}
		original := make([]float32, len(input))
		copy(original, input)

		Normalize(input)

		for i, v := range input {
			if v != original[i] {
				t.Errorf("Normalize modified input at index %d", i)
			}
		}
	})
}

func TestNormalizeInPlace(t *testing.T) {
	t.Run("empty vector", func(t *testing.T) {
		mag := NormalizeInPlace([]float32{})
		if mag != 0 {
			t.Errorf("NormalizeInPlace([]) magnitude = %f, want 0", mag)
		}
	})

	t.Run("zero vector", func(t *testing.T) {
		vec := []float32{0, 0, 0}
		mag := NormalizeInPlace(vec)
		if mag != 0 {
			t.Errorf("NormalizeInPlace([0,0,0]) magnitude = %f, want 0", mag)
		}
	})

	t.Run("modifies input", func(t *testing.T) {
		vec := []float32{3, 4, 0}
		mag := NormalizeInPlace(vec)

		if !floatEqual(mag, 5) {
			t.Errorf("returned magnitude = %f, want 5", mag)
		}
		if !floatEqual(vec[0], 0.6) {
			t.Errorf("vec[0] = %f, want 0.6", vec[0])
		}
		if !floatEqual(vec[1], 0.8) {
			t.Errorf("vec[1] = %f, want 0.8", vec[1])
		}
	})
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("empty vectors", func(t *testing.T) {
		result := CosineSimilarity([]float32{}, []float32{})
		if result != 0 {
			t.Errorf("CosineSimilarity([], []) = %f, want 0", result)
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		result := CosineSimilarity([]float32{1, 2}, []float32{1, 2, 3})
		if result != 0 {
			t.Errorf("CosineSimilarity with mismatched dims = %f, want 0", result)
		}
	})

	t.Run("zero vector", func(t *testing.T) {
		result := CosineSimilarity([]float32{1, 0, 0}, []float32{0, 0, 0})
		if result != 0 {
			t.Errorf("CosineSimilarity with zero vector = %f, want 0", result)
		}
	})

	t.Run("identical vectors", func(t *testing.T) {
		result := CosineSimilarity([]float32{1, 2, 3}, []float32{1, 2, 3})
		if !floatEqual(result, 1) {
			t.Errorf("CosineSimilarity(v, v) = %f, want 1", result)
		}
	})

	t.Run("opposite vectors", func(t *testing.T) {
		result := CosineSimilarity([]float32{1, 0, 0}, []float32{-1, 0, 0})
		if !floatEqual(result, -1) {
			t.Errorf("CosineSimilarity(v, -v) = %f, want -1", result)
		}
	})

	t.Run("orthogonal vectors", func(t *testing.T) {
		result := CosineSimilarity([]float32{1, 0, 0}, []float32{0, 1, 0})
		if !floatEqual(result, 0) {
			t.Errorf("CosineSimilarity of orthogonal vectors = %f, want 0", result)
		}
	})

	t.Run("45 degree angle", func(t *testing.T) {
		// cos(45°) ≈ 0.707
		a := []float32{1, 0}
		b := []float32{1, 1}
		result := CosineSimilarity(a, b)
		expected := float32(1 / math.Sqrt(2))
		if !floatEqual(result, expected) {
			t.Errorf("CosineSimilarity at 45° = %f, want ~%f", result, expected)
		}
	})
}

func TestDotProduct(t *testing.T) {
	t.Run("dimension mismatch", func(t *testing.T) {
		result := DotProduct([]float32{1, 2}, []float32{1, 2, 3})
		if result != 0 {
			t.Errorf("DotProduct with mismatched dims = %f, want 0", result)
		}
	})

	t.Run("zero vector", func(t *testing.T) {
		result := DotProduct([]float32{1, 2, 3}, []float32{0, 0, 0})
		if result != 0 {
			t.Errorf("DotProduct with zero vector = %f, want 0", result)
		}
	})

	t.Run("unit vectors", func(t *testing.T) {
		result := DotProduct([]float32{1, 0, 0}, []float32{1, 0, 0})
		if !floatEqual(result, 1) {
			t.Errorf("DotProduct of unit vectors = %f, want 1", result)
		}
	})

	t.Run("general vectors", func(t *testing.T) {
		// [1, 2, 3] · [4, 5, 6] = 4 + 10 + 18 = 32
		result := DotProduct([]float32{1, 2, 3}, []float32{4, 5, 6})
		if !floatEqual(result, 32) {
			t.Errorf("DotProduct([1,2,3], [4,5,6]) = %f, want 32", result)
		}
	})
}

func TestEuclideanDistance(t *testing.T) {
	t.Run("dimension mismatch", func(t *testing.T) {
		result := EuclideanDistance([]float32{1, 2}, []float32{1, 2, 3})
		if result != -1 {
			t.Errorf("EuclideanDistance with mismatched dims = %f, want -1", result)
		}
	})

	t.Run("same point", func(t *testing.T) {
		result := EuclideanDistance([]float32{1, 2, 3}, []float32{1, 2, 3})
		if result != 0 {
			t.Errorf("EuclideanDistance of same point = %f, want 0", result)
		}
	})

	t.Run("unit distance", func(t *testing.T) {
		result := EuclideanDistance([]float32{0, 0, 0}, []float32{1, 0, 0})
		if !floatEqual(result, 1) {
			t.Errorf("EuclideanDistance = %f, want 1", result)
		}
	})

	t.Run("3-4-5 triangle", func(t *testing.T) {
		result := EuclideanDistance([]float32{0, 0}, []float32{3, 4})
		if !floatEqual(result, 5) {
			t.Errorf("EuclideanDistance = %f, want 5", result)
		}
	})
}

func TestManhattanDistance(t *testing.T) {
	t.Run("dimension mismatch", func(t *testing.T) {
		result := ManhattanDistance([]float32{1, 2}, []float32{1, 2, 3})
		if result != -1 {
			t.Errorf("ManhattanDistance with mismatched dims = %f, want -1", result)
		}
	})

	t.Run("same point", func(t *testing.T) {
		result := ManhattanDistance([]float32{1, 2, 3}, []float32{1, 2, 3})
		if result != 0 {
			t.Errorf("ManhattanDistance of same point = %f, want 0", result)
		}
	})

	t.Run("general vectors", func(t *testing.T) {
		// |1-4| + |2-6| + |3-2| = 3 + 4 + 1 = 8
		result := ManhattanDistance([]float32{1, 2, 3}, []float32{4, 6, 2})
		if !floatEqual(result, 8) {
			t.Errorf("ManhattanDistance = %f, want 8", result)
		}
	})
}

func TestCosineDistance(t *testing.T) {
	t.Run("identical vectors", func(t *testing.T) {
		result := CosineDistance([]float32{1, 2, 3}, []float32{1, 2, 3})
		if !floatEqual(result, 0) {
			t.Errorf("CosineDistance of identical vectors = %f, want 0", result)
		}
	})

	t.Run("opposite vectors", func(t *testing.T) {
		result := CosineDistance([]float32{1, 0, 0}, []float32{-1, 0, 0})
		if !floatEqual(result, 2) {
			t.Errorf("CosineDistance of opposite vectors = %f, want 2", result)
		}
	})
}

func TestAdd(t *testing.T) {
	t.Run("dimension mismatch", func(t *testing.T) {
		result := Add([]float32{1, 2}, []float32{1, 2, 3})
		if result != nil {
			t.Error("Add with mismatched dims should return nil")
		}
	})

	t.Run("general vectors", func(t *testing.T) {
		result := Add([]float32{1, 2, 3}, []float32{4, 5, 6})
		expected := []float32{5, 7, 9}
		for i, v := range result {
			if v != expected[i] {
				t.Errorf("result[%d] = %f, want %f", i, v, expected[i])
			}
		}
	})
}

func TestSubtract(t *testing.T) {
	t.Run("dimension mismatch", func(t *testing.T) {
		result := Subtract([]float32{1, 2}, []float32{1, 2, 3})
		if result != nil {
			t.Error("Subtract with mismatched dims should return nil")
		}
	})

	t.Run("general vectors", func(t *testing.T) {
		result := Subtract([]float32{4, 5, 6}, []float32{1, 2, 3})
		expected := []float32{3, 3, 3}
		for i, v := range result {
			if v != expected[i] {
				t.Errorf("result[%d] = %f, want %f", i, v, expected[i])
			}
		}
	})
}

func TestScale(t *testing.T) {
	t.Run("scale by zero", func(t *testing.T) {
		result := Scale([]float32{1, 2, 3}, 0)
		for i, v := range result {
			if v != 0 {
				t.Errorf("result[%d] = %f, want 0", i, v)
			}
		}
	})

	t.Run("scale by one", func(t *testing.T) {
		input := []float32{1, 2, 3}
		result := Scale(input, 1)
		for i, v := range result {
			if v != input[i] {
				t.Errorf("result[%d] = %f, want %f", i, v, input[i])
			}
		}
	})

	t.Run("scale by two", func(t *testing.T) {
		result := Scale([]float32{1, 2, 3}, 2)
		expected := []float32{2, 4, 6}
		for i, v := range result {
			if v != expected[i] {
				t.Errorf("result[%d] = %f, want %f", i, v, expected[i])
			}
		}
	})

	t.Run("scale by negative", func(t *testing.T) {
		result := Scale([]float32{1, 2, 3}, -1)
		expected := []float32{-1, -2, -3}
		for i, v := range result {
			if v != expected[i] {
				t.Errorf("result[%d] = %f, want %f", i, v, expected[i])
			}
		}
	})
}

func TestMean(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		result := Mean([][]float32{})
		if result != nil {
			t.Error("Mean([]) should return nil")
		}
	})

	t.Run("empty vectors", func(t *testing.T) {
		result := Mean([][]float32{{}})
		if result != nil {
			t.Error("Mean([[]]) should return nil")
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		result := Mean([][]float32{{1, 2}, {1, 2, 3}})
		if result != nil {
			t.Error("Mean with mismatched dims should return nil")
		}
	})

	t.Run("single vector", func(t *testing.T) {
		result := Mean([][]float32{{1, 2, 3}})
		expected := []float32{1, 2, 3}
		for i, v := range result {
			if v != expected[i] {
				t.Errorf("result[%d] = %f, want %f", i, v, expected[i])
			}
		}
	})

	t.Run("multiple vectors", func(t *testing.T) {
		vectors := [][]float32{
			{1, 2, 3},
			{3, 4, 5},
		}
		result := Mean(vectors)
		expected := []float32{2, 3, 4}
		for i, v := range result {
			if !floatEqual(v, expected[i]) {
				t.Errorf("result[%d] = %f, want %f", i, v, expected[i])
			}
		}
	})
}

func TestFindTopK(t *testing.T) {
	t.Run("k <= 0", func(t *testing.T) {
		result := FindTopK([]float32{1, 0}, [][]float32{{1, 0}}, 0)
		if result != nil {
			t.Error("FindTopK with k=0 should return nil")
		}
	})

	t.Run("empty vectors", func(t *testing.T) {
		result := FindTopK([]float32{1, 0}, [][]float32{}, 5)
		if result != nil {
			t.Error("FindTopK with empty vectors should return nil")
		}
	})

	t.Run("k > len(vectors)", func(t *testing.T) {
		query := []float32{1, 0}
		vectors := [][]float32{{1, 0}, {0, 1}}
		result := FindTopK(query, vectors, 10)
		if len(result) != 2 {
			t.Errorf("FindTopK with k>n returned %d results, want 2", len(result))
		}
	})

	t.Run("correct order", func(t *testing.T) {
		query := []float32{1, 0}
		vectors := [][]float32{
			{0, 1},  // Orthogonal, similarity 0
			{1, 0},  // Identical, similarity 1
			{-1, 0}, // Opposite, similarity -1
		}
		result := FindTopK(query, vectors, 3)

		if result[0].Index != 1 || !floatEqual(result[0].Similarity, 1) {
			t.Errorf("Top result: index=%d, similarity=%f, want index=1, similarity=1",
				result[0].Index, result[0].Similarity)
		}
		if result[1].Index != 0 || !floatEqual(result[1].Similarity, 0) {
			t.Errorf("Second result: index=%d, similarity=%f, want index=0, similarity=0",
				result[1].Index, result[1].Similarity)
		}
		if result[2].Index != 2 || !floatEqual(result[2].Similarity, -1) {
			t.Errorf("Third result: index=%d, similarity=%f, want index=2, similarity=-1",
				result[2].Index, result[2].Similarity)
		}
	})
}

func TestIsNormalized(t *testing.T) {
	t.Run("empty vector", func(t *testing.T) {
		if IsNormalized([]float32{}) {
			t.Error("IsNormalized([]) should return false")
		}
	})

	t.Run("zero vector", func(t *testing.T) {
		if IsNormalized([]float32{0, 0, 0}) {
			t.Error("IsNormalized([0,0,0]) should return false")
		}
	})

	t.Run("normalized vector", func(t *testing.T) {
		if !IsNormalized([]float32{1, 0, 0}) {
			t.Error("IsNormalized([1,0,0]) should return true")
		}
	})

	t.Run("non-normalized vector", func(t *testing.T) {
		if IsNormalized([]float32{1, 1, 0}) {
			t.Error("IsNormalized([1,1,0]) should return false")
		}
	})

	t.Run("after normalization", func(t *testing.T) {
		v := Normalize([]float32{3, 4, 0})
		if !IsNormalized(v) {
			t.Error("Normalized vector should pass IsNormalized")
		}
	})
}

func TestZeroVector(t *testing.T) {
	t.Run("zero dimension", func(t *testing.T) {
		result := ZeroVector(0)
		if len(result) != 0 {
			t.Errorf("ZeroVector(0) length = %d, want 0", len(result))
		}
	})

	t.Run("positive dimension", func(t *testing.T) {
		result := ZeroVector(5)
		if len(result) != 5 {
			t.Errorf("ZeroVector(5) length = %d, want 5", len(result))
		}
		for i, v := range result {
			if v != 0 {
				t.Errorf("ZeroVector[%d] = %f, want 0", i, v)
			}
		}
	})
}

func TestRandomVector(t *testing.T) {
	t.Run("returns correct dimension", func(t *testing.T) {
		result := RandomVector(100, 42)
		if len(result) != 100 {
			t.Errorf("RandomVector(100, 42) length = %d, want 100", len(result))
		}
	})

	t.Run("is normalized", func(t *testing.T) {
		result := RandomVector(100, 42)
		if !IsNormalized(result) {
			t.Error("RandomVector should return normalized vector")
		}
	})

	t.Run("deterministic with seed", func(t *testing.T) {
		v1 := RandomVector(10, 42)
		v2 := RandomVector(10, 42)
		for i := range v1 {
			if v1[i] != v2[i] {
				t.Error("RandomVector with same seed should return same result")
				break
			}
		}
	})

	t.Run("different with different seed", func(t *testing.T) {
		v1 := RandomVector(10, 42)
		v2 := RandomVector(10, 43)
		same := true
		for i := range v1 {
			if v1[i] != v2[i] {
				same = false
				break
			}
		}
		if same {
			t.Error("RandomVector with different seeds should return different results")
		}
	})
}

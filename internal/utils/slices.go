package utils

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64
}

func FilterFunc[S ~[]E, E any](s S, f func(E) bool) []E {
	var result []E
	for _, v := range s {
		if f(v) {
			result = append(result, v)
		}
	}

	return result
}

func Map[S ~[]E, E, R any](s S, f func(E) R) []R {
	result := make([]R, len(s))
	for i, v := range s {
		result[i] = f(v)
	}

	return result
}

func Avg[S ~[]E, E Number](s S) E {
	var sum E
	for _, v := range s {
		sum += v
	}

	return sum / E(len(s))
}

func Reduce[S ~[]E, E, R any](s S, f func(R, E) R, init R) R {
	result := init
	for _, v := range s {
		result = f(result, v)
	}

	return result
}

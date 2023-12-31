package utils

func MapValue[T float32 | float64](value, fromMin, fromMax, toMin, toMax T) T {
	return (value-fromMin)/(fromMax-fromMin)*(toMax-toMin) + toMin
}

package utils

import "testing"

const (
	benchSizeSmall  = 16
	benchSizeMedium = 64
	benchSizeLarge  = 256
)

func BenchmarkRandomString_Small(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = RandomString(benchSizeSmall)
	}
}

func BenchmarkRandomString_Medium(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = RandomString(benchSizeMedium)
	}
}

func BenchmarkRandomString_Large(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = RandomString(benchSizeLarge)
	}
}

func BenchmarkRandomString_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = RandomString(benchSizeMedium)
		}
	})
}

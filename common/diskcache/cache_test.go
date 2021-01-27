package diskcache

// func init() {
// 	for i := 0; i < 1e3; i++ {
// 		ioutil.WriteFile("xx"+strconv.Itoa(i), []byte("hellow"), 0777)
// 	}
//
// }
//
// func BenchmarkRead(b *testing.B) {
// 	for i := 0; i < b.N; i++ {
// 		f, err := os.Open(".")
// 		if err != nil {
// 			b.Fatal(err)
// 		}
// 		f.Readdir(-1)
// 		f.Close()
// 	}
// }
//
// func BenchmarkRead2(b *testing.B) {
// 	for i := 0; i < b.N; i++ {
// 		f, err := os.Open(".")
// 		if err != nil {
// 			b.Fatal(err)
// 		}
// 		f.Readdirnames(-1)
// 		f.Close()
// 	}
// }

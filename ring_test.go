package sanguo

//go test -run=.
//go tool cover -html=coverage.out
import (
	"log"
	"testing"
)

func TestRing(t *testing.T) {
	r := ring{data: make([]interface{}, 10)}
	for i := 1; i <= 10; i++ {
		r.push(i)
	}
	log.Println(r)

	for i := 1; i <= 10; i++ {
		log.Println(r.pop())
	}

	log.Println(r)

	for i := 1; i <= 10; i++ {
		r.push(i)
	}
	log.Println(r)

	r.push(11)

	log.Println(r)

	for i := 1; i <= 10; i++ {
		log.Println(r.pop())
	}
	log.Println(r.pop())
	log.Println(r)

	r.push(12)
	log.Println(r)

}

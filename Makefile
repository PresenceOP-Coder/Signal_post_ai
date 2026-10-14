.PHONY: all run test score sample clean

all: test run score

run:
	go run ./cmd/signalpost/main.go run --input orgnrs.txt --out profiles/

test:
	go test -v ./...

score:
	go run ./cmd/scorer/main.go --profiles profiles --report profiles/run_report.json

sample:
	go run ./cmd/sample/main.go -count 1000 -out orgnrs_1000.txt

clean:
	rm -rf profiles/

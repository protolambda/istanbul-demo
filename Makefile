bin:
	mkdir -p bin

bin/mips-program.elf: bin
	env GO111MODULE=on GOOS=linux GOARCH=mips GOMIPS=softfloat go build -v -o ./bin/mips-program.elf ./client
	# verify output with: readelf -h bin/op-program-client.elf
	# result is mips32, big endian, R3000

bin/prestate.json: bin/mips-program.elf
	cannon load-elf --path ./bin/mips-program.elf --out bin/prestate.json --meta bin/meta.json

bin/program-server: bin
	go build -o bin/program-server ./server

bin/proof: bin
	mkdir -p bin/proof

proof_example: bin/prestate.json bin/program-server bin/proof
	cannon run --info-at='%10' --proof-at '=123' \
		--stop-at '=123' --input ./bin/prestate.json \
		--meta bin/meta.json --proof-fmt 'bin/proof/%d.json' \
		--output "bin/out.json" -- ./bin/program-server



bin:
	mkdir -p bin

bin/mips-program.elf: bin
	env GO111MODULE=on GOOS=linux GOARCH=mips GOMIPS=softfloat go build -v -o ./bin/mips-program.elf ./client
	# verify output with: readelf -h bin/op-program-client.elf
	# result is mips32, big endian, R3000

bin/cannon:
	cd ../optimism && go build -o ../istanbul-demo/bin/cannon ./cannon

bin/prestate.json: bin/cannon bin/mips-program.elf
	./bin/cannon load-elf --path ./bin/mips-program.elf --out bin/prestate.json --meta bin/meta.json

bin/program-server: bin
	go build -o bin/program-server ./server

bin/proof: bin
	mkdir -p bin/proof

proof_example: bin/cannon bin/prestate.json bin/program-server bin/proof
	./bin/cannon run --info-at='%10' --proof-at '=123' \
		--stop-at '=123' --input ./bin/prestate.json \
		--meta bin/meta.json --proof-fmt 'bin/proof/%d.json' \
		--output "bin/out.json" -- ./bin/program-server

bin/client: bin
	go build -v -o ./bin/client ./client

bin/server: bin
	go build -v -o ./bin/server ./server

bin/oracle: bin
	cd ../optimism && go build -o ../istanbul-demo/bin/oracle ./op-preimage/cmd/oracle

oracle_test: bin/oracle bin/client bin/server
	./bin/oracle \
		--client="./bin/client" --host="./bin/server" \
		--info.hints \
		--info.preimage-keys --info.preimage-values


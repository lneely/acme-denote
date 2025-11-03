install:V:
	go build -o $HOME/bin/Denote ./cmd/Denote

clean:V:
	rm -f $HOME/bin/Denote

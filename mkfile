install:V:
	go build -o $HOME/bin/Denote ./cmd/Denote
	go build -o $HOME/bin/Journal ./cmd/Journal

clean:V:
	rm -f $HOME/bin/Denote $HOME/bin/Journal

install:V:
	go build -o $HOME/bin/Denote .
	go build -o $HOME/bin/Drename cmd/Drename/main.go
	cp scripts/Djournal $HOME/bin/Djournal
	cp scripts/Dmerge $HOME/bin/Dmerge

clean:V:
	rm -f $HOME/bin/Denote $HOME/bin/Drename $HOME/bin/Djournal $HOME/bin/Dmerge
install:V:
	go build -o $HOME/bin/Denote .
	cp scripts/Djournal $HOME/bin/Djournal
	cp scripts/Dmerge $HOME/bin/Dmerge

clean:V:
	rm -f $HOME/bin/Denote $HOME/bin/Djournal $HOME/bin/Dmerge
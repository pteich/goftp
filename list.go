package goftp

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const timeFormat = "20060102150405"

type FileType int

const (
	DIR FileType = iota
	FILE
)

type FtpFileInfo struct {
	Name  string
	Size  int64
	Type  FileType
	Mtime time.Time
	Ctime time.Time
	Raw   string
}

// List lists the path (or current directory)
func (ftp *FTP) List(path string) (files []string, err error) {
	if err = ftp.Type(TypeASCII); err != nil {
		return
	}

	var port int
	if port, err = ftp.Pasv(); err != nil {
		return
	}

	// check if MLSD works
	if err = ftp.send("MLSD %s", path); err != nil {
	}

	var pconn net.Conn
	if pconn, err = ftp.newConnection(port); err != nil {
		return
	}
	defer pconn.Close()

	var line string
	if line, err = ftp.receiveNoDiscard(); err != nil {
		return
	}

	if !strings.HasPrefix(line, StatusFileOK) {
		// MLSD failed, lets try LIST
		if err = ftp.send("LIST %s", path); err != nil {
			return
		}

		if line, err = ftp.receiveNoDiscard(); err != nil {
			return
		}

		if !strings.HasPrefix(line, StatusFileOK) {
			// Really list is not working here
			err = errors.New(line)
			return
		}
	}

	reader := bufio.NewReader(pconn)

	for {
		line, err = reader.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return
		}

		files = append(files, string(line))
	}
	// Must close for vsftp tlsed conenction otherwise does not receive connection
	pconn.Close()

	if line, err = ftp.receive(); err != nil {
		return
	}

	if !strings.HasPrefix(line, StatusClosingDataConnection) {
		err = errors.New(line)
		return
	}

	return
}

func (ftp *FTP) ListParsed(path string) (info []FtpFileInfo, err error) {
	files, err := ftp.List(path)
	if err != nil {
		return nil, err
	}

	for i := range files {
		var size int64

		fileinfo := FtpFileInfo{}

		facts := make(map[string]string)
		data := strings.Split(files[i], ";")

		fileinfo.Name = strings.TrimSpace(data[len(data)-1])
		fileinfo.Raw = files[i]

		for ii := range data {
			fields := strings.Split(data[ii], "=")
			if 2 == len(fields) {
				facts[strings.ToLower(fields[0])] = strings.ToLower(fields[1])
			}
		}
		if facts["type"] == "" {
			continue
		}
		if facts["type"] == "dir" || facts["type"] == "cdir" || facts["type"] == "pdir" {
			fileinfo.Type = DIR
		} else {
			fileinfo.Type = FILE
		}

		if facts["size"] != "" {
			size, err = strconv.ParseInt(facts["size"], 10, 64)
		} else if fileinfo.Type == DIR && facts["sizd"] != "" {
			size, err = strconv.ParseInt(facts["sizd"], 10, 64)
		}

		fileinfo.Size = size

		if facts["modify"] != "" {
			if mtime, err := time.ParseInLocation(timeFormat, facts["modify"], time.UTC); err == nil {
				fileinfo.Mtime = mtime
			}
		}
		if facts["create"] != "" {
			if ctime, err := time.ParseInLocation(timeFormat, facts["create"], time.UTC); err == nil {
				fileinfo.Ctime = ctime
			}
		}

		info = append(info, fileinfo)
	}

	return info, nil
}

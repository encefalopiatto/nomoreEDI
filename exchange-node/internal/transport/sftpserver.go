package transport

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// StartInboundSFTP runs a small SFTP server whose only purpose is receiving
// supermessage files: partners log in with a username and password from the
// users map and upload into what they see as the current folder — which is
// really the node's own transport/in directory, where the pipeline picks
// every file up. Nothing outside that directory is ever visible or writable.
func StartInboundSFTP(addr string, users map[string]string, hostSigner ssh.Signer, inDir string) error {
	config := SFTPServerConfig(users, hostSigner)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return ServeSFTP(listener, config, inDir)
}

// ServeSFTP accepts connections on an existing listener (tests use this to
// pick a free port first).
func ServeSFTP(listener net.Listener, config *ssh.ServerConfig, inDir string) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go handleSFTPConn(conn, config, inDir)
	}
}

// SFTPServerConfig builds the ssh server config for the drop folder.
func SFTPServerConfig(users map[string]string, hostSigner ssh.Signer) *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if want, ok := users[conn.User()]; ok && want != "" && string(pass) == want {
				return nil, nil
			}
			return nil, fmt.Errorf("unknown user or wrong password")
		},
	}
	config.AddHostKey(hostSigner)
	return config
}

func handleSFTPConn(conn net.Conn, config *ssh.ServerConfig, inDir string) {
	defer conn.Close()
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "only sftp sessions are served")
			continue
		}
		channel, requests, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func(in <-chan *ssh.Request) {
			for req := range in {
				ok := req.Type == "subsystem" && len(req.Payload) > 4 && string(req.Payload[4:]) == "sftp"
				req.Reply(ok, nil)
			}
		}(requests)
		server := sftp.NewRequestServer(channel, dropboxHandlers(inDir))
		server.Serve()
		server.Close()
	}
}

// dropboxHandlers confines every operation to flat file names inside inDir.
func dropboxHandlers(inDir string) sftp.Handlers {
	h := &dropbox{inDir: inDir}
	return sftp.Handlers{FileGet: h, FilePut: h, FileCmd: h, FileList: h}
}

type dropbox struct {
	inDir string
	mu    sync.Mutex
}

// resolve flattens any client-supplied path to a bare file name inside the
// drop folder — path tricks cannot escape it.
func (d *dropbox) resolve(p string) string {
	return filepath.Join(d.inDir, filepath.Base(strings.ReplaceAll(p, "\\", "/")))
}

func (d *dropbox) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return os.OpenFile(d.resolve(r.Filepath), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}

func (d *dropbox) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	return nil, errors.New("this server only receives files")
}

func (d *dropbox) Filecmd(r *sftp.Request) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch r.Method {
	case "Rename":
		return os.Rename(d.resolve(r.Filepath), d.resolve(r.Target))
	case "Remove":
		return os.Remove(d.resolve(r.Filepath))
	case "Setstat":
		return nil // permissions/timestamps are irrelevant in the drop folder
	default:
		return fmt.Errorf("operation %q is not offered here", r.Method)
	}
}

func (d *dropbox) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	switch r.Method {
	case "Stat":
		info, err := os.Stat(d.resolve(r.Filepath))
		if err != nil {
			// The drop folder itself must always stat, so clients can cd into it.
			if r.Filepath == "/" || r.Filepath == "." {
				return listerOf(fakeDir{}), nil
			}
			return nil, err
		}
		return listerOf(info), nil
	case "List":
		return listerOf(fakeDir{}), nil // uploads are not listable back
	default:
		return nil, fmt.Errorf("operation %q is not offered here", r.Method)
	}
}

type fakeDir struct{}

func (fakeDir) Name() string       { return "." }
func (fakeDir) Size() int64        { return 0 }
func (fakeDir) Mode() os.FileMode  { return os.ModeDir | 0o755 }
func (fakeDir) ModTime() time.Time { return time.Unix(0, 0) }
func (fakeDir) IsDir() bool        { return true }
func (fakeDir) Sys() any           { return nil }

func listerOf(infos ...os.FileInfo) sftp.ListerAt {
	return listerAt(infos)
}

type listerAt []os.FileInfo

func (l listerAt) ListAt(f []os.FileInfo, off int64) (int, error) {
	if off >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(f, l[off:])
	if off+int64(n) >= int64(len(l)) {
		return n, io.EOF
	}
	return n, nil
}

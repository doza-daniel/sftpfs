package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sshfs/filesystem"
	"strings"
	"syscall"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const envPassword = "SFTPFS_PASSWORD"
const envUsername = "SFTPFS_USERNAME"

func main() {
	flagMountpoint := flag.String("m", "/tmp/mnt", "Directory where the fs should be mounted.")
	flagUsername := flag.String("u", "", "Username.")
	flagPasswordPrompt := flag.Bool("p", false, "Password prompt.")
	flagServerHost := flag.String("server", "alas.math.rs", "Host of the remote SSH server.")
	flagServerPort := flag.Int("port", 22, "Port of the remote SSH server.")
	flag.Parse()

	password, err := getPassword(*flagPasswordPrompt, os.Getenv(envPassword))
	if err != nil {
		log.Fatalf("failed to read password: %v", err)
	}

	username, err := getUsername(*flagUsername, os.Getenv(envUsername))
	if err != nil {
		log.Fatalf("failed to read username: %v", err)
	}

	sftpClient, err := setUpSftp(username, password, *flagServerHost, *flagServerPort)
	if err != nil {
		log.Fatalf("failed to setup SFTP client: %v", err)
	}

	srv := fuseutil.NewFileSystemServer(filesystem.New(sftpClient))

	mfs, err := fuse.Mount(*flagMountpoint, srv, &fuse.MountConfig{})
	if err != nil {
		log.Fatalf("mount failed: %v", err)
	}

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		mountpoint := mfs.Dir()
		if err := fuse.Unmount(mountpoint); err != nil {
			log.Fatalf("failed to unmount %s: %v", mountpoint, err)
		}
	}()

	if err := mfs.Join(context.Background()); err != nil {
		log.Fatalf("Join error: %v", err)
	}
}

func setUpSftp(username, password, host string, port int) (*sftp.Client, error) {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %v: %v", addr, err)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, err
	}

	return sftpClient, nil
}

func getUsername(fromFlag string, def string) (string, error) {
	if len(fromFlag) > 0 {
		return fromFlag, nil
	}

	if len(def) == 0 {
		return "", fmt.Errorf("default username is empty")
	}

	return def, nil
}

func getPassword(prompt bool, def string) (string, error) {
	if !prompt {
		if def == "" {
			return "", fmt.Errorf("default password is empty")
		}

		return def, nil
	}

	fmt.Print("Enter Password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	password := string(bytePassword)
	return strings.TrimSpace(password), nil
}

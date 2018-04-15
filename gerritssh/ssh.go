package gerritssh

import "golang.org/x/crypto/ssh"

// Client holds the necessary params to connect to a gerrit instance over
// ssh
type Client struct {
	privateKey ssh.Signer
	hostKey    ssh.PublicKey
	user       string
	addr       string
}

// NewClient returns a new SSHClient
func NewClient(sshAddr, user string, privateKey, hostKey []byte) (*Client, error) {
	k, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	hk, _, _, _, err := ssh.ParseAuthorizedKey(hostKey)
	if err != nil {
		return nil, err
	}

	return &Client{
		privateKey: k,
		hostKey:    hk,
		user:       user,
		addr:       sshAddr,
	}, nil
}

// Dial connects to gerrit over ssh and returns a new session
func (s Client) Dial() (*ssh.Session, error) {
	cfg := &ssh.ClientConfig{
		User: s.user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(s.privateKey),
		},
		HostKeyCallback:   ssh.FixedHostKey(s.hostKey),
		HostKeyAlgorithms: []string{s.hostKey.Type()},
	}
	c, err := ssh.Dial("tcp", s.addr, cfg)
	if err != nil {
		return nil, err
	}
	return c.NewSession()
}

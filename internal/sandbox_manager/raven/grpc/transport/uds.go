package transport

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	"github.com/ssuji15/wolf/internal/util"
	"google.golang.org/grpc"
)

type UDSTransport struct {
	SocketPath string
}

func NewUDSTransport(workDir string) (*UDSTransport, error) {
	path := fmt.Sprintf("%s/socket.sock", workDir)
	if err := util.EnsureDirExist(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return &UDSTransport{
		SocketPath: path,
	}, nil
}

func (t *UDSTransport) Dial(ctx context.Context) (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial("unix", t.SocketPath)
	}

	return grpc.DialContext(
		ctx,
		"unix://"+t.SocketPath,
		grpc.WithInsecure(),
		grpc.WithContextDialer(dialer),
	)
}

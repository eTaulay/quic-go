package http3

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/quicvarint"
)

type connection struct {
	session quic.EarlySession

	settings Settings

	peerSettingsDone chan struct{} // Closed when peer settings are read
	peerSettings     Settings
	peerSettingsErr  error

	peerStreamsMutex sync.Mutex
	peerStreams      [4]quic.ReceiveStream
}

var (
	_ Conn       = &connection{}
	_ ClientConn = &connection{}
	_ ServerConn = &connection{}
)

// Accept establishes a new HTTP/3 server connection from an existing QUIC session.
// If settings is nil, it will use a set of reasonable defaults.
func Accept(s quic.EarlySession, settings Settings) (ServerConn, error) {
	if s.Perspective() != quic.PerspectiveServer {
		return nil, errors.New("Accept called on client session")
	}
	return newConn(s, settings)
}

// Open establishes a new HTTP/3 client connection from an existing QUIC session.
// If settings is nil, it will use a set of reasonable defaults.
func Open(s quic.EarlySession, settings Settings) (ClientConn, error) {
	if s.Perspective() != quic.PerspectiveClient {
		return nil, errors.New("Open called on server session")
	}
	return newConn(s, settings)
}

func newConn(s quic.EarlySession, settings Settings) (*connection, error) {
	if settings == nil {
		settings = Settings{}
		// TODO: this blocks, so is this too clever?
		if s.ConnectionState().SupportsDatagrams {
			settings.EnableDatagrams()
		}
	}

	conn := &connection{
		session:          s,
		settings:         settings,
		peerSettingsDone: make(chan struct{}),
	}

	str, err := conn.session.OpenUniStream()
	if err != nil {
		return nil, err
	}
	w := quicvarint.NewWriter(str)
	quicvarint.Write(w, uint64(StreamTypeControl))
	conn.settings.writeFrame(w)

	go conn.handleIncomingUniStreams()

	return conn, nil
}

func (conn *connection) handleIncomingUniStreams() {
	for {
		str, err := conn.session.AcceptUniStream(context.Background())
		if err != nil {
			// TODO: log the error
			return
		}
		go conn.handleIncomingUniStream(str)
	}
}

func (conn *connection) handleIncomingUniStream(str quic.ReceiveStream) {
	r := quicvarint.NewReader(str)
	t, err := quicvarint.Read(r)
	if err != nil {
		str.CancelRead(quic.StreamErrorCode(errorGeneralProtocolError))
		return
	}
	streamType := StreamType(t)

	// Store control, QPACK, and push streams on conn
	if streamType < 4 {
		conn.peerStreamsMutex.Lock()
		if conn.peerStreams[streamType] != nil {
			conn.session.CloseWithError(quic.ApplicationErrorCode(errorStreamCreationError), fmt.Sprintf("more than one %s opened", streamType))
			return
		}
		conn.peerStreams[streamType] = str
		conn.peerStreamsMutex.Unlock()
	}

	switch streamType {
	case StreamTypeControl:
		conn.handleControlStream(str)
	case StreamTypePush:
		if conn.session.Perspective() == quic.PerspectiveServer {
			conn.session.CloseWithError(quic.ApplicationErrorCode(errorStreamCreationError), fmt.Sprintf("spurious %s from client", streamType))
			return
		}
		// TODO: handle push streams
		// We never increased the Push ID, so we don't expect any push streams.
		conn.session.CloseWithError(quic.ApplicationErrorCode(errorIDError), "MAX_PUSH_ID = 0")
		return
	case StreamTypeQPACKEncoder, StreamTypeQPACKDecoder:
		// TODO: handle QPACK dynamic tables
	default:
		// TODO: demultiplex incoming uni streams
		str.CancelRead(quic.StreamErrorCode(errorStreamCreationError))
		// conn.incomingUniStreams <- str
	}
}

func (conn *connection) handleControlStream(str quic.ReceiveStream) {
	f, err := parseNextFrame(str)
	if err != nil {
		conn.session.CloseWithError(quic.ApplicationErrorCode(errorFrameError), "")
		return
	}
	settings, ok := f.(Settings)
	if !ok {
		err := &quic.ApplicationError{
			ErrorCode: quic.ApplicationErrorCode(errorMissingSettings),
		}
		conn.session.CloseWithError(err.ErrorCode, err.ErrorMessage)
		conn.peerSettingsErr = err
		return
	}
	// If datagram support was enabled on this side and the peer side, we can expect it to have been
	// negotiated both on the transport and on the HTTP/3 layer.
	// Note: ConnectionState() will block until the handshake is complete (relevant when using 0-RTT).
	if settings.DatagramsEnabled() && !conn.session.ConnectionState().SupportsDatagrams {
		err := &quic.ApplicationError{
			ErrorCode:    quic.ApplicationErrorCode(errorSettingsError),
			ErrorMessage: "missing QUIC Datagram support",
		}
		conn.session.CloseWithError(err.ErrorCode, err.ErrorMessage)
		conn.peerSettingsErr = err
		return
	}
	conn.peerSettings = settings
	close(conn.peerSettingsDone)

	// TODO: loop reading the reset of the frames from the control stream
}

// TODO: demultiplex incoming bidi streams
func (conn *connection) AcceptRequestStream(ctx context.Context) (RequestStream, error) {
	if conn.session.Perspective() != quic.PerspectiveServer {
		return nil, errors.New("server method called on client connection")
	}
	str, err := conn.session.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return newRequestStream(conn, str)
}

// TODO: multiplex outgoing bidi streams?
func (conn *connection) OpenRequestStream(ctx context.Context) (RequestStream, error) {
	if conn.session.Perspective() != quic.PerspectiveClient {
		return nil, errors.New("client method called on server connection")
	}
	str, err := conn.session.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return newRequestStream(conn, str)
}

func (conn *connection) Settings() Settings {
	return conn.settings
}

func (conn *connection) PeerSettings() (Settings, error) {
	select {
	case <-conn.peerSettingsDone:
		return conn.peerSettings, conn.peerSettingsErr
	case <-conn.session.Context().Done():
		return nil, conn.session.Context().Err()
	}
}

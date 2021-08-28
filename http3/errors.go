package http3

import (
	"fmt"

	"github.com/lucas-clemente/quic-go"
)

type errorCode quic.ApplicationErrorCode

const (
	errorNoError              errorCode = 0x100
	errorGeneralProtocolError errorCode = 0x101
	errorInternalError        errorCode = 0x102
	errorStreamCreationError  errorCode = 0x103
	errorClosedCriticalStream errorCode = 0x104
	errorFrameUnexpected      errorCode = 0x105
	errorFrameError           errorCode = 0x106
	errorExcessiveLoad        errorCode = 0x107
	errorIDError              errorCode = 0x108
	errorSettingsError        errorCode = 0x109
	errorMissingSettings      errorCode = 0x10a
	errorRequestRejected      errorCode = 0x10b
	errorRequestCanceled      errorCode = 0x10c
	errorRequestIncomplete    errorCode = 0x10d
	errorMessageError         errorCode = 0x10e
	errorConnectError         errorCode = 0x10f
	errorVersionFallback      errorCode = 0x110

	// https://www.ietf.org/archive/id/draft-ietf-webtrans-http3-01.html#section-7.5
	errorWebTransportBufferedStreamRejected errorCode = 0x3994bd84
)

func (e errorCode) String() string {
	switch e {
	case errorNoError:
		return "H3_NO_ERROR"
	case errorGeneralProtocolError:
		return "H3_GENERAL_PROTOCOL_ERROR"
	case errorInternalError:
		return "H3_INTERNAL_ERROR"
	case errorStreamCreationError:
		return "H3_STREAM_CREATION_ERROR"
	case errorClosedCriticalStream:
		return "H3_CLOSED_CRITICAL_STREAM"
	case errorFrameUnexpected:
		return "H3_FRAME_UNEXPECTED"
	case errorFrameError:
		return "H3_FRAME_ERROR"
	case errorExcessiveLoad:
		return "H3_EXCESSIVE_LOAD"
	case errorIDError:
		return "H3_ID_ERROR"
	case errorSettingsError:
		return "H3_SETTINGS_ERROR"
	case errorMissingSettings:
		return "H3_MISSING_SETTINGS"
	case errorRequestRejected:
		return "H3_REQUEST_REJECTED"
	case errorRequestCanceled:
		return "H3_REQUEST_CANCELLED"
	case errorRequestIncomplete:
		return "H3_INCOMPLETE_REQUEST"
	case errorMessageError:
		return "H3_MESSAGE_ERROR"
	case errorConnectError:
		return "H3_CONNECT_ERROR"
	case errorVersionFallback:
		return "H3_VERSION_FALLBACK"
	case errorWebTransportBufferedStreamRejected:
		return "H3_WEBTRANSPORT_BUFFERED_STREAM_REJECTED"
	default:
		return fmt.Sprintf("unknown error code: %#x", uint64(e))
	}
}

type frameTypeError struct {
	Want FrameType
	Type FrameType
	Len  uint64
}

func (err *frameTypeError) Error() string {
	return fmt.Sprintf("unexpected frame type %s, expected %s", err.Type, err.Want)
}

var _ error = &frameTypeError{}

type frameLengthError struct {
	FrameType FrameType
	Length    uint64
	Max       uint64
}

var _ error = &frameLengthError{}

func (err *frameLengthError) Error() string {
	return fmt.Sprintf("%s frame too large: %d bytes (max: %d)", err.FrameType, err.Length, err.Max)
}

type connError struct {
	Code errorCode
	Err  error
}

var _ error = &connError{}

func (err *connError) Error() string {
	return fmt.Sprintf("connection error %s: %s", err.Code, err.Err)
}

func (err *connError) Unwrap() error {
	return err.Err
}

type streamError struct {
	Code errorCode
	Err  error
}

var _ error = &streamError{}

func (err *streamError) Error() string {
	return fmt.Sprintf("stream error %s: %s", err.Code, err.Err)
}

func (err *streamError) Unwrap() error {
	return err.Err
}
package main

import (
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sower-proxy/deferlog/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type forwardTarget struct {
	Namespace string
	Pods      []string
	Port      int32
}

func (f *forwardTarget) forword(conn net.Conn) (err error) {
	if len(f.Pods) == 0 {
		return errors.New("No pods to forward to")
	}

	var streamConn httpstream.Connection
	for _, pod := range f.Pods {
		streamConn, err = dialPod(f.Namespace, pod)
		if err == nil {
			break
		}
	}
	if err != nil {
		return errors.Wrap(err, "failed to dial pod")
	}

	headers := http.Header{}
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	headers.Set(corev1.PortHeader, strconv.Itoa(int(f.Port)))
	headers.Set(corev1.PortForwardRequestIDHeader, strconv.Itoa(0))

	log.Info().Interface("stream", streamConn).Interface("headers", headers).Msg("Forwarding")
	errorStream, err := streamConn.CreateStream(headers)
	if err != nil {
		return errors.Wrap(err, "Could not create stream")
	}
	// we're not writing to this stream
	errorStream.Close()

	go func() {
		message, err := io.ReadAll(errorStream)
		log.InfoWarn(err).
			Str("message", string(message)).
			Msgf("read from error stream")
	}()

	headers.Set(corev1.StreamType, corev1.StreamTypeData)
	dataStream, err := streamConn.CreateStream(headers)
	if err != nil {
		return errors.Wrap(err, "Could not create stream")
	}
	defer dataStream.Close()

	go func() {
		_, err := io.Copy(dataStream, conn)
		log.InfoWarn(err).
			Msgf("copy from conn to data stream")
	}()

	_, err = io.Copy(conn, dataStream)
	log.InfoWarn(err).
		Msgf("copy from data stream to conn")

	return err
}

func dialPod(ns, pod string) (httpstream.Connection, error) {
	restConf, cli := getClient()

	url := cli.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(ns).
		Name(pod).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(restConf)
	if err != nil {
		return nil, errors.Wrap(err, "Could not create round tripper")
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)
	streamConn, _, err := dialer.Dial(portforward.PortForwardProtocolV1Name)
	return streamConn, err
}

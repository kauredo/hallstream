package web

import (
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/kauredo/hallstream/internal/cable"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/store"
)

// maxPort is the longest port name kept: COM12, /dev/cu.usbserial-A10K3L2Z.
const maxPort = 200

type serialReply struct {
	source.SerialStatus
	Ports []string `json:"ports"`
}

func (s *Server) serialRoutes() {
	s.mux.HandleFunc("GET /api/serial", func(w http.ResponseWriter, r *http.Request) {
		out := serialReply{Ports: []string{}}
		if s.Serial != nil {
			out.SerialStatus = s.Serial.Status()
		} else {
			set := s.Store.Settings().Serial
			out.Port, out.Protocol = set.Port, set.Protocol
		}
		if s.SerialPorts != nil {
			ports, err := s.SerialPorts()
			if err != nil {
				s.Log(i18n.T("log.serial.sem_lista", "err", err))
			}
			if ports != nil {
				out.Ports = ports
			}
		}
		reply(w, out)
	})
}

// cleanSerial keeps a known protocol and a port name of sane length. A phone
// keeps the saved port and protocol: both belong to the PC.
func cleanSerial(req *store.Serial, cur store.Serial, onPC bool) {
	if !onPC || len(req.Port) > maxPort || strings.ContainsFunc(req.Port, unicode.IsControl) {
		req.Port = cur.Port
	}
	if !onPC {
		req.Protocol = cur.Protocol
	}
	if !slices.Contains(cable.Names, req.Protocol) {
		req.Protocol = cable.Names[0]
	}
}

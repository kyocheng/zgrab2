// Package mssql provides the zgrab2 scanner module for the MSSQL protocol.
// Default Port: 1433 (TCP)
//
// The --encrypt-mode flag allows setting an explicit client encryption mode
// (the default is ENCRYPT_ON). Note: only ENCRYPT_NOT_SUP will skip the TLS
// handshake, since even ENCRYPT_OFF uses TLS for the login step.
//
// The scan performs a PRELOGIN and if possible does a TLS handshake.
//
// The output is the the server version and instance name, and if applicable the
// TLS output.
package mssql

import (
	log "github.com/sirupsen/logrus"
	"github.com/zmap/zgrab2"
)

// ScanResults contains detailed information about each step of the
// MySQL handshake, and can be encoded to JSON.
type ScanResults struct {
	// Version is the version returned by the server in the PRELOGIN response.
	// Its format is "MAJOR.MINOR.BUILD_NUMBER".
	Version string `json:"version,omitempty"`

	// InstanceName is the value of the INSTANCE field returned by the server
	// in the PRELOGIN response.
	InstanceName string `json:"instance_name,omitempty"`

	// PreloginOptions are the raw key-value pairs returned by the server in
	// response to the PRELOGIN call. Debug only.
	PreloginOptions *PreloginOptions `json:"prelogin_options,omitempty" zgrab:"debug"`

	// TLSLog is the shared TLS handshake/scan log.
	TLSLog *zgrab2.TLSLog `json:"tls,omitempty"`
}

// Flags defines the command-line configuration options for the module.
type Flags struct {
	zgrab2.BaseFlags
	zgrab2.TLSFlags
	EncryptMode string `long:"encrypt-mode" description:"The type of encryption to request in the pre-login step. One of ENCRYPT_ON, ENCRYPT_OFF, ENCRYPT_NOT_SUP." default:"ENCRYPT_ON"`
	Verbose     bool   `long:"verbose" description:"More verbose logging, include debug fields in the scan results"`
}

// Module is the implementation of zgrab2.Module for the MSSQL protocol.
type Module struct {
}

// Scanner is the implementation of zgrab2.Scanner for the MSSQL protocol.
type Scanner struct {
	config *Flags
}

// NewFlags returns a default Flags instance to be populated by the command
// line flags.
func (module *Module) NewFlags() interface{} {
	return new(Flags)
}

// NewScanner returns a new Scanner instance.
func (module *Module) NewScanner() zgrab2.Scanner {
	return new(Scanner)
}

// Validate does nothing in this module.
func (flags *Flags) Validate(args []string) error {
	return nil
}

// Help returns the help string for this module.
func (flags *Flags) Help() string {
	return ""
}

// Init initializes the Scanner instance with the given command-line flags.
func (scanner *Scanner) Init(flags zgrab2.ScanFlags) error {
	f, _ := flags.(*Flags)
	scanner.config = f
	return nil
}

// InitPerSender does nothing in this module.
func (scanner *Scanner) InitPerSender(senderID int) error {
	return nil
}

// GetName returns the configured scanner name.
func (scanner *Scanner) GetName() string {
	return scanner.config.Name
}

// GetPort returns the configured scanner port.
func (scanner *Scanner) GetPort() uint {
	return scanner.config.Port
}

// Scan performs the MSSQL scan.
// 1. Open a TCP connection to the target port (default 1433).
// 2. Send a PRELOGIN packet to the server.
// 3. Read the PRELOGIN response from the server.
// 4. If the server encrypt mode is EncryptModeNotSupported, break.
// 5. Perform a TLS handshake, with the packets wrapped in TDS headers.
// 6. Decode the Version and InstanceName from the PRELOGIN response
func (scanner *Scanner) Scan(target zgrab2.ScanTarget) (zgrab2.ScanStatus, interface{}, error) {
	conn, err := target.Open(&scanner.config.BaseFlags)
	if err != nil {
		return zgrab2.TryGetScanStatus(err), nil, err
	}
	sql := NewConnection(conn)
	defer sql.Close()
	result := &ScanResults{}
	_, err = sql.Handshake(scanner.config)

	if sql.tlsConn != nil {
		result.TLSLog = sql.tlsConn.GetLog()
	}

	if sql.PreloginOptions != nil {
		result.PreloginOptions = sql.PreloginOptions
		version := sql.PreloginOptions.GetVersion()
		if version != nil {
			result.Version = version.String()
		}
		result.InstanceName = string((*sql.PreloginOptions)[PreloginInstance])
	}

	if err != nil {
		if sql.PreloginOptions == nil && sql.readValidTDSPacket == false {
			// If we received no PreloginOptions and none of the packets we've
			// read appeared to be a valid TDS header, then the inference is
			// that we found no MSSQL service on the target.
			// NOTE: In the case where PreloginOptions == nil but
			// readValidTDSPacket == true, the result will be empty, but not
			// nil.
			result = nil
		}
		switch err {
		case ErrNoServerEncryption:
			return zgrab2.SCAN_APPLICATION_ERROR, result, err
		case ErrServerRequiresEncryption:
			return zgrab2.SCAN_APPLICATION_ERROR, result, err
		default:
			return zgrab2.TryGetScanStatus(err), result, err
		}
	}
	return zgrab2.SCAN_SUCCESS, result, nil
}

// RegisterModule is called by modules/mssql.go's init()
func RegisterModule() {
	var module Module
	_, err := zgrab2.AddCommand("mssql", "MSSQL", "Grab a mssql handshake", 1433, &module)
	log.SetLevel(log.DebugLevel)
	if err != nil {
		log.Fatal(err)
	}
}

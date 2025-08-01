// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Copyright (C) 2015-2022 The Lightning Network Developers

package lnd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	flags "github.com/jessevdk/go-flags"
	"github.com/lightninglabs/neutrino"
	"github.com/lightningnetwork/lnd/autopilot"
	"github.com/lightningnetwork/lnd/build"
	"github.com/lightningnetwork/lnd/chainreg"
	"github.com/lightningnetwork/lnd/chanbackup"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/discovery"
	"github.com/lightningnetwork/lnd/funding"
	"github.com/lightningnetwork/lnd/htlcswitch"
	"github.com/lightningnetwork/lnd/htlcswitch/hodl"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/peersrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/signrpc"
	"github.com/lightningnetwork/lnd/lnutils"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/lightningnetwork/lnd/tor"
)

const (
	defaultDataDirname        = "data"
	defaultChainSubDirname    = "chain"
	defaultGraphSubDirname    = "graph"
	defaultTowerSubDirname    = "watchtower"
	defaultTLSCertFilename    = "tls.cert"
	defaultTLSKeyFilename     = "tls.key"
	defaultAdminMacFilename   = "admin.macaroon"
	defaultReadMacFilename    = "readonly.macaroon"
	defaultInvoiceMacFilename = "invoice.macaroon"
	defaultLogLevel           = "info"
	defaultLogDirname         = "logs"
	defaultLogFilename        = "lnd.log"
	defaultRPCPort            = 10009
	defaultRESTPort           = 8080
	defaultPeerPort           = 9735
	defaultRPCHost            = "localhost"

	defaultNoSeedBackup                  = false
	defaultPaymentsExpirationGracePeriod = time.Duration(0)
	defaultTrickleDelay                  = 90 * 1000
	defaultChanStatusSampleInterval      = time.Minute
	defaultChanEnableTimeout             = 19 * time.Minute
	defaultChanDisableTimeout            = 20 * time.Minute
	defaultHeightHintCacheQueryDisable   = false
	defaultMinBackoff                    = time.Second
	defaultMaxBackoff                    = time.Hour
	defaultLetsEncryptDirname            = "letsencrypt"
	defaultLetsEncryptListen             = ":80"

	defaultTorSOCKSPort            = 9050
	defaultTorDNSHost              = "soa.nodes.lightning.directory"
	defaultTorDNSPort              = 53
	defaultTorControlPort          = 9051
	defaultTorV2PrivateKeyFilename = "v2_onion_private_key"
	defaultTorV3PrivateKeyFilename = "v3_onion_private_key"

	// defaultZMQReadDeadline is the default read deadline to be used for
	// both the block and tx ZMQ subscriptions.
	defaultZMQReadDeadline = 5 * time.Second

	// DefaultAutogenValidity is the default validity of a self-signed
	// certificate. The value corresponds to 14 months
	// (14 months * 30 days * 24 hours).
	defaultTLSCertDuration = 14 * 30 * 24 * time.Hour

	// minTimeLockDelta is the minimum timelock we require for incoming
	// HTLCs on our channels.
	minTimeLockDelta = routing.MinCLTVDelta

	// MaxTimeLockDelta is the maximum CLTV delta that can be applied to
	// forwarded HTLCs.
	MaxTimeLockDelta = routing.MaxCLTVDelta

	// defaultAcceptorTimeout is the time after which an RPCAcceptor will time
	// out and return false if it hasn't yet received a response.
	defaultAcceptorTimeout = 15 * time.Second

	defaultAlias = ""
	defaultColor = "#3399FF"

	// defaultCoopCloseTargetConfs is the default confirmation target
	// that will be used to estimate a fee rate to use during a
	// cooperative channel closure initiated by a remote peer. By default
	// we'll set this to a lax value since we weren't the ones that
	// initiated the channel closure.
	defaultCoopCloseTargetConfs = 6

	// defaultBlockCacheSize is the size (in bytes) of blocks that will be
	// keep in memory if no size is specified.
	defaultBlockCacheSize uint64 = 20 * 1024 * 1024 // 20 MB

	// defaultHostSampleInterval is the default amount of time that the
	// HostAnnouncer will wait between DNS resolutions to check if the
	// backing IP of a host has changed.
	defaultHostSampleInterval = time.Minute * 5

	defaultChainInterval = time.Minute
	defaultChainTimeout  = time.Second * 30
	defaultChainBackoff  = time.Minute * 2
	defaultChainAttempts = 3

	// Set defaults for a health check which ensures that we have space
	// available on disk. Although this check is off by default so that we
	// avoid breaking any existing setups (particularly on mobile), we still
	// set the other default values so that the health check can be easily
	// enabled with sane defaults.
	defaultRequiredDisk = 0.1
	defaultDiskInterval = time.Hour * 12
	defaultDiskTimeout  = time.Second * 5
	defaultDiskBackoff  = time.Minute
	defaultDiskAttempts = 0

	// Set defaults for a health check which ensures that the TLS certificate
	// is not expired. Although this check is off by default (not all setups
	// require it), we still set the other default values so that the health
	// check can be easily enabled with sane defaults.
	defaultTLSInterval = time.Minute
	defaultTLSTimeout  = time.Second * 5
	defaultTLSBackoff  = time.Minute
	defaultTLSAttempts = 0

	// Set defaults for a health check which ensures that the tor
	// connection is alive. Although this check is off by default (not all
	// setups require it), we still set the other default values so that
	// the health check can be easily enabled with sane defaults.
	defaultTCInterval = time.Minute
	defaultTCTimeout  = time.Second * 5
	defaultTCBackoff  = time.Minute
	defaultTCAttempts = 0

	// Set defaults for a health check which ensures that the remote signer
	// RPC connection is alive. Although this check is off by default (only
	// active when remote signing is turned on), we still set the other
	// default values so that the health check can be easily enabled with
	// sane defaults.
	defaultRSInterval = time.Minute
	defaultRSTimeout  = time.Second * 1
	defaultRSBackoff  = time.Second * 30
	defaultRSAttempts = 1

	// Set defaults for a health check which ensures that the leader
	// election is functioning correctly. Although this check is off by
	// default (as etcd leader election is only used in a clustered setup),
	// we still set the default values so that the health check can be
	// easily enabled with sane defaults. Note that by default we only run
	// this check once, as it is critical for the node's operation.
	defaultLeaderCheckInterval = time.Minute
	defaultLeaderCheckTimeout  = time.Second * 5
	defaultLeaderCheckBackoff  = time.Second * 5
	defaultLeaderCheckAttempts = 1

	// defaultRemoteMaxHtlcs specifies the default limit for maximum
	// concurrent HTLCs the remote party may add to commitment transactions.
	// This value can be overridden with --default-remote-max-htlcs.
	defaultRemoteMaxHtlcs = 483

	// defaultMaxLocalCSVDelay is the maximum delay we accept on our
	// commitment output. The local csv delay maximum is now equal to
	// the remote csv delay maximum we require for the remote commitment
	// transaction.
	defaultMaxLocalCSVDelay = 2016

	// defaultChannelCommitInterval is the default maximum time between
	// receiving a channel state update and signing a new commitment.
	defaultChannelCommitInterval = 50 * time.Millisecond

	// maxChannelCommitInterval is the maximum time the commit interval can
	// be configured to.
	maxChannelCommitInterval = time.Hour

	// defaultPendingCommitInterval specifies the default timeout value
	// while waiting for the remote party to revoke a locally initiated
	// commitment state.
	defaultPendingCommitInterval = 1 * time.Minute

	// maxPendingCommitInterval specifies the max allowed duration when
	// waiting for the remote party to revoke a locally initiated
	// commitment state.
	maxPendingCommitInterval = 5 * time.Minute

	// defaultChannelCommitBatchSize is the default maximum number of
	// channel state updates that is accumulated before signing a new
	// commitment.
	defaultChannelCommitBatchSize = 10

	// defaultCoinSelectionStrategy is the coin selection strategy that is
	// used by default to fund transactions.
	defaultCoinSelectionStrategy = "largest"

	// defaultKeepFailedPaymentAttempts is the default setting for whether
	// to keep failed payments in the database.
	defaultKeepFailedPaymentAttempts = false

	// defaultGrpcServerPingTime is the default duration for the amount of
	// time of no activity after which the server pings the client to see if
	// the transport is still alive. If set below 1s, a minimum value of 1s
	// will be used instead.
	defaultGrpcServerPingTime = time.Minute

	// defaultGrpcServerPingTimeout is the default duration the server waits
	// after having pinged for keepalive check, and if no activity is seen
	// even after that the connection is closed.
	defaultGrpcServerPingTimeout = 20 * time.Second

	// defaultGrpcClientPingMinWait is the default minimum amount of time a
	// client should wait before sending a keepalive ping.
	defaultGrpcClientPingMinWait = 5 * time.Second

	// defaultHTTPHeaderTimeout is the default timeout for HTTP requests.
	DefaultHTTPHeaderTimeout = 5 * time.Second

	// DefaultNumRestrictedSlots is the default max number of incoming
	// connections allowed in the server. Outbound connections are not
	// restricted.
	DefaultNumRestrictedSlots = 100

	// BitcoinChainName is a string that represents the Bitcoin blockchain.
	BitcoinChainName = "bitcoin"

	bitcoindBackendName = "bitcoind"
	btcdBackendName     = "btcd"
	neutrinoBackendName = "neutrino"

	defaultPrunedNodeMaxPeers = 4
	defaultNeutrinoMaxPeers   = 8

	// defaultNoDisconnectOnPongFailure is the default value for whether we
	// should *not* disconnect from a peer if we don't receive a pong
	// response in time after we send a ping.
	defaultNoDisconnectOnPongFailure = false
)

var (
	// DefaultLndDir is the default directory where lnd tries to find its
	// configuration file and store its data. This is a directory in the
	// user's application data, for example:
	//   C:\Users\<username>\AppData\Local\Lnd on Windows
	//   ~/.lnd on Linux
	//   ~/Library/Application Support/Lnd on MacOS
	DefaultLndDir = btcutil.AppDataDir("lnd", false)

	// DefaultConfigFile is the default full path of lnd's configuration
	// file.
	DefaultConfigFile = filepath.Join(DefaultLndDir, lncfg.DefaultConfigFilename)

	defaultDataDir = filepath.Join(DefaultLndDir, defaultDataDirname)
	defaultLogDir  = filepath.Join(DefaultLndDir, defaultLogDirname)

	defaultTowerDir = filepath.Join(defaultDataDir, defaultTowerSubDirname)

	defaultTLSCertPath    = filepath.Join(DefaultLndDir, defaultTLSCertFilename)
	defaultTLSKeyPath     = filepath.Join(DefaultLndDir, defaultTLSKeyFilename)
	defaultLetsEncryptDir = filepath.Join(DefaultLndDir, defaultLetsEncryptDirname)

	defaultBtcdDir         = btcutil.AppDataDir(btcdBackendName, false)
	defaultBtcdRPCCertFile = filepath.Join(defaultBtcdDir, "rpc.cert")

	defaultBitcoindDir = btcutil.AppDataDir(BitcoinChainName, false)

	defaultTorSOCKS   = net.JoinHostPort("localhost", strconv.Itoa(defaultTorSOCKSPort))
	defaultTorDNS     = net.JoinHostPort(defaultTorDNSHost, strconv.Itoa(defaultTorDNSPort))
	defaultTorControl = net.JoinHostPort("localhost", strconv.Itoa(defaultTorControlPort))

	// bitcoindEsimateModes defines all the legal values for bitcoind's
	// estimatesmartfee RPC call.
	defaultBitcoindEstimateMode = "CONSERVATIVE"
	bitcoindEstimateModes       = [2]string{"ECONOMICAL", defaultBitcoindEstimateMode}
)

// Config defines the configuration options for lnd.
//
// See LoadConfig for further details regarding the configuration
// loading+parsing process.
//
//nolint:ll
type Config struct {
	ShowVersion bool `short:"V" long:"version" description:"Display version information and exit"`

	LndDir       string `long:"lnddir" description:"The base directory that contains lnd's data, logs, configuration file, etc. This option overwrites all other directory options."`
	ConfigFile   string `short:"C" long:"configfile" description:"Path to configuration file"`
	DataDir      string `short:"b" long:"datadir" description:"The directory to store lnd's data within"`
	SyncFreelist bool   `long:"sync-freelist" description:"Whether the databases used within lnd should sync their freelist to disk. This is disabled by default resulting in improved memory performance during operation, but with an increase in startup time."`

	TLSCertPath        string        `long:"tlscertpath" description:"Path to write the TLS certificate for lnd's RPC and REST services"`
	TLSKeyPath         string        `long:"tlskeypath" description:"Path to write the TLS private key for lnd's RPC and REST services"`
	TLSExtraIPs        []string      `long:"tlsextraip" description:"Adds an extra ip to the generated certificate"`
	TLSExtraDomains    []string      `long:"tlsextradomain" description:"Adds an extra domain to the generated certificate"`
	TLSAutoRefresh     bool          `long:"tlsautorefresh" description:"Re-generate TLS certificate and key if the IPs or domains are changed"`
	TLSDisableAutofill bool          `long:"tlsdisableautofill" description:"Do not include the interface IPs or the system hostname in TLS certificate, use first --tlsextradomain as Common Name instead, if set"`
	TLSCertDuration    time.Duration `long:"tlscertduration" description:"The duration for which the auto-generated TLS certificate will be valid for"`
	TLSEncryptKey      bool          `long:"tlsencryptkey" description:"Automatically encrypts the TLS private key and generates ephemeral TLS key pairs when the wallet is locked or not initialized"`

	NoMacaroons     bool          `long:"no-macaroons" description:"Disable macaroon authentication, can only be used if server is not listening on a public interface."`
	AdminMacPath    string        `long:"adminmacaroonpath" description:"Path to write the admin macaroon for lnd's RPC and REST services if it doesn't exist"`
	ReadMacPath     string        `long:"readonlymacaroonpath" description:"Path to write the read-only macaroon for lnd's RPC and REST services if it doesn't exist"`
	InvoiceMacPath  string        `long:"invoicemacaroonpath" description:"Path to the invoice-only macaroon for lnd's RPC and REST services if it doesn't exist"`
	LogDir          string        `long:"logdir" description:"Directory to log output."`
	MaxLogFiles     int           `long:"maxlogfiles" description:"Maximum logfiles to keep (0 for no rotation). DEPRECATED: use --logging.file.max-files instead" hidden:"true"`
	MaxLogFileSize  int           `long:"maxlogfilesize" description:"Maximum logfile size in MB. DEPRECATED: use --logging.file.max-file-size instead" hidden:"true"`
	AcceptorTimeout time.Duration `long:"acceptortimeout" description:"Time after which an RPCAcceptor will time out and return false if it hasn't yet received a response"`

	LetsEncryptDir    string `long:"letsencryptdir" description:"The directory to store Let's Encrypt certificates within"`
	LetsEncryptListen string `long:"letsencryptlisten" description:"The IP:port on which lnd will listen for Let's Encrypt challenges. Let's Encrypt will always try to contact on port 80. Often non-root processes are not allowed to bind to ports lower than 1024. This configuration option allows a different port to be used, but must be used in combination with port forwarding from port 80. This configuration can also be used to specify another IP address to listen on, for example an IPv6 address."`
	LetsEncryptDomain string `long:"letsencryptdomain" description:"Request a Let's Encrypt certificate for this domain. Note that the certificate is only requested and stored when the first rpc connection comes in."`

	// We'll parse these 'raw' string arguments into real net.Addrs in the
	// loadConfig function. We need to expose the 'raw' strings so the
	// command line library can access them.
	// Only the parsed net.Addrs should be used!
	RawRPCListeners   []string `long:"rpclisten" description:"Add an interface/port/socket to listen for RPC connections"`
	RawRESTListeners  []string `long:"restlisten" description:"Add an interface/port/socket to listen for REST connections"`
	RawListeners      []string `long:"listen" description:"Add an interface/port to listen for peer connections"`
	RawExternalIPs    []string `long:"externalip" description:"Add an ip:port to the list of local addresses we claim to listen on to peers. If a port is not specified, the default (9735) will be used regardless of other parameters"`
	ExternalHosts     []string `long:"externalhosts" description:"Add a hostname:port that should be periodically resolved to announce IPs for. If a port is not specified, the default (9735) will be used."`
	RPCListeners      []net.Addr
	RESTListeners     []net.Addr
	RestCORS          []string `long:"restcors" description:"Add an ip:port/hostname to allow cross origin access from. To allow all origins, set as \"*\"."`
	Listeners         []net.Addr
	ExternalIPs       []net.Addr
	DisableListen     bool          `long:"nolisten" description:"Disable listening for incoming peer connections"`
	DisableRest       bool          `long:"norest" description:"Disable REST API"`
	DisableRestTLS    bool          `long:"no-rest-tls" description:"Disable TLS for REST connections"`
	WSPingInterval    time.Duration `long:"ws-ping-interval" description:"The ping interval for REST based WebSocket connections, set to 0 to disable sending ping messages from the server side"`
	WSPongWait        time.Duration `long:"ws-pong-wait" description:"The time we wait for a pong response message on REST based WebSocket connections before the connection is closed as inactive"`
	NAT               bool          `long:"nat" description:"Toggle NAT traversal support (using either UPnP or NAT-PMP) to automatically advertise your external IP address to the network -- NOTE this does not support devices behind multiple NATs"`
	AddPeers          []string      `long:"addpeer" description:"Specify peers to connect to first"`
	MinBackoff        time.Duration `long:"minbackoff" description:"Shortest backoff when reconnecting to persistent peers. Valid time units are {s, m, h}."`
	MaxBackoff        time.Duration `long:"maxbackoff" description:"Longest backoff when reconnecting to persistent peers. Valid time units are {s, m, h}."`
	ConnectionTimeout time.Duration `long:"connectiontimeout" description:"The timeout value for network connections. Valid time units are {ms, s, m, h}."`

	DebugLevel string `short:"d" long:"debuglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <global-level>,<subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems -- Use show to list available subsystems"`

	CPUProfile      string `long:"cpuprofile" description:"DEPRECATED: Use 'pprof.cpuprofile' option. Write CPU profile to the specified file" hidden:"true"`
	Profile         string `long:"profile" description:"DEPRECATED: Use 'pprof.profile' option. Enable HTTP profiling on either a port or host:port" hidden:"true"`
	BlockingProfile int    `long:"blockingprofile" description:"DEPRECATED: Use 'pprof.blockingprofile' option. Used to enable a blocking profile to be served on the profiling port. This takes a value from 0 to 1, with 1 including every blocking event, and 0 including no events." hidden:"true"`
	MutexProfile    int    `long:"mutexprofile" description:"DEPRECATED: Use 'pprof.mutexprofile' option. Used to Enable a mutex profile to be served on the profiling port. This takes a value from 0 to 1, with 1 including every mutex event, and 0 including no events." hidden:"true"`

	Pprof *lncfg.Pprof `group:"Pprof" namespace:"pprof"`

	UnsafeDisconnect   bool   `long:"unsafe-disconnect" description:"DEPRECATED: Allows the rpcserver to intentionally disconnect from peers with open channels. THIS FLAG WILL BE REMOVED IN 0.10.0" hidden:"true"`
	UnsafeReplay       bool   `long:"unsafe-replay" description:"Causes a link to replay the adds on its commitment txn after starting up, this enables testing of the sphinx replay logic."`
	MaxPendingChannels int    `long:"maxpendingchannels" description:"The maximum number of incoming pending channels permitted per peer."`
	BackupFilePath     string `long:"backupfilepath" description:"The target location of the channel backup file"`

	NoBackupArchive bool `long:"no-backup-archive" description:"If set to true, channel backups will be deleted or replaced rather than being archived to a separate location."`

	FeeURL string `long:"feeurl" description:"DEPRECATED: Use 'fee.url' option. Optional URL for external fee estimation. If no URL is specified, the method for fee estimation will depend on the chosen backend and network. Must be set for neutrino on mainnet." hidden:"true"`

	Bitcoin      *lncfg.Chain    `group:"Bitcoin" namespace:"bitcoin"`
	BtcdMode     *lncfg.Btcd     `group:"btcd" namespace:"btcd"`
	BitcoindMode *lncfg.Bitcoind `group:"bitcoind" namespace:"bitcoind"`
	NeutrinoMode *lncfg.Neutrino `group:"neutrino" namespace:"neutrino"`

	BlockCacheSize uint64 `long:"blockcachesize" description:"The maximum capacity of the block cache"`

	Autopilot *lncfg.AutoPilot `group:"Autopilot" namespace:"autopilot"`

	Tor *lncfg.Tor `group:"Tor" namespace:"tor"`

	SubRPCServers *subRPCServerConfigs `group:"subrpc"`

	Hodl *hodl.Config `group:"hodl" namespace:"hodl"`

	NoNetBootstrap bool `long:"nobootstrap" description:"If true, then automatic network bootstrapping will not be attempted."`

	NoSeedBackup             bool   `long:"noseedbackup" description:"If true, NO SEED WILL BE EXPOSED -- EVER, AND THE WALLET WILL BE ENCRYPTED USING THE DEFAULT PASSPHRASE. THIS FLAG IS ONLY FOR TESTING AND SHOULD NEVER BE USED ON MAINNET."`
	WalletUnlockPasswordFile string `long:"wallet-unlock-password-file" description:"The full path to a file (or pipe/device) that contains the password for unlocking the wallet; if set, no unlocking through RPC is possible and lnd will exit if no wallet exists or the password is incorrect; if wallet-unlock-allow-create is also set then lnd will ignore this flag if no wallet exists and allow a wallet to be created through RPC."`
	WalletUnlockAllowCreate  bool   `long:"wallet-unlock-allow-create" description:"Don't fail with an error if wallet-unlock-password-file is set but no wallet exists yet."`

	ResetWalletTransactions bool `long:"reset-wallet-transactions" description:"Removes all transaction history from the on-chain wallet on startup, forcing a full chain rescan starting at the wallet's birthday. Implements the same functionality as btcwallet's dropwtxmgr command. Should be set to false after successful execution to avoid rescanning on every restart of lnd."`

	CoinSelectionStrategy string `long:"coin-selection-strategy" description:"The strategy to use for selecting coins for wallet transactions." choice:"largest" choice:"random"`

	PaymentsExpirationGracePeriod time.Duration `long:"payments-expiration-grace-period" description:"A period to wait before force closing channels with outgoing htlcs that have timed-out and are a result of this node initiated payments."`
	TrickleDelay                  int           `long:"trickledelay" description:"Time in milliseconds between each release of announcements to the network"`
	ChanEnableTimeout             time.Duration `long:"chan-enable-timeout" description:"The duration that a peer connection must be stable before attempting to send a channel update to re-enable or cancel a pending disables of the peer's channels on the network."`
	ChanDisableTimeout            time.Duration `long:"chan-disable-timeout" description:"The duration that must elapse after first detecting that an already active channel is actually inactive and sending channel update disabling it to the network. The pending disable can be canceled if the peer reconnects and becomes stable for chan-enable-timeout before the disable update is sent."`
	ChanStatusSampleInterval      time.Duration `long:"chan-status-sample-interval" description:"The polling interval between attempts to detect if an active channel has become inactive due to its peer going offline."`
	HeightHintCacheQueryDisable   bool          `long:"height-hint-cache-query-disable" description:"Disable queries from the height-hint cache to try to recover channels stuck in the pending close state. Disabling height hint queries may cause longer chain rescans, resulting in a performance hit. Unset this after channels are unstuck so you can get better performance again."`
	Alias                         string        `long:"alias" description:"The node alias. Used as a moniker by peers and intelligence services"`
	Color                         string        `long:"color" description:"The color of the node in hex format (i.e. '#3399FF'). Used to customize node appearance in intelligence services"`
	MinChanSize                   int64         `long:"minchansize" description:"The smallest channel size (in satoshis) that we should accept. Incoming channels smaller than this will be rejected"`
	MaxChanSize                   int64         `long:"maxchansize" description:"The largest channel size (in satoshis) that we should accept. Incoming channels larger than this will be rejected"`
	CoopCloseTargetConfs          uint32        `long:"coop-close-target-confs" description:"The target number of blocks that a cooperative channel close transaction should confirm in. This is used to estimate the fee to use as the lower bound during fee negotiation for the channel closure."`

	ChannelCommitInterval time.Duration `long:"channel-commit-interval" description:"The maximum time that is allowed to pass between receiving a channel state update and signing the next commitment. Setting this to a longer duration allows for more efficient channel operations at the cost of latency."`

	PendingCommitInterval time.Duration `long:"pending-commit-interval" description:"The maximum time that is allowed to pass while waiting for the remote party to revoke a locally initiated commitment state. Setting this to a longer duration if a slow response is expected from the remote party or large number of payments are attempted at the same time."`

	ChannelCommitBatchSize uint32 `long:"channel-commit-batch-size" description:"The maximum number of channel state updates that is accumulated before signing a new commitment."`

	KeepFailedPaymentAttempts bool `long:"keep-failed-payment-attempts" description:"Keeps persistent record of all failed payment attempts for successfully settled payments."`

	StoreFinalHtlcResolutions bool `long:"store-final-htlc-resolutions" description:"Persistently store the final resolution of incoming htlcs."`

	DefaultRemoteMaxHtlcs uint16 `long:"default-remote-max-htlcs" description:"The default max_htlc applied when opening or accepting channels. This value limits the number of concurrent HTLCs that the remote party can add to the commitment. The maximum possible value is 483."`

	NumGraphSyncPeers      int           `long:"numgraphsyncpeers" description:"The number of peers that we should receive new graph updates from. This option can be tuned to save bandwidth for light clients or routing nodes."`
	HistoricalSyncInterval time.Duration `long:"historicalsyncinterval" description:"The polling interval between historical graph sync attempts. Each historical graph sync attempt ensures we reconcile with the remote peer's graph from the genesis block."`

	IgnoreHistoricalGossipFilters bool `long:"ignore-historical-gossip-filters" description:"If true, will not reply with historical data that matches the range specified by a remote peer's gossip_timestamp_filter. Doing so will result in lower memory and bandwidth requirements."`

	RejectPush bool `long:"rejectpush" description:"If true, lnd will not accept channel opening requests with non-zero push amounts. This should prevent accidental pushes to merchant nodes."`

	RejectHTLC bool `long:"rejecthtlc" description:"If true, lnd will not forward any HTLCs that are meant as onward payments. This option will still allow lnd to send HTLCs and receive HTLCs but lnd won't be used as a hop."`

	AcceptPositiveInboundFees bool `long:"accept-positive-inbound-fees" description:"If true, lnd will also allow setting positive inbound fees. By default, lnd only allows to set negative inbound fees (an inbound \"discount\") to remain backwards compatible with senders whose implementations do not yet support inbound fees."`

	// RequireInterceptor determines whether the HTLC interceptor is
	// registered regardless of whether the RPC is called or not.
	RequireInterceptor bool `long:"requireinterceptor" description:"Whether to always intercept HTLCs, even if no stream is attached"`

	StaggerInitialReconnect bool `long:"stagger-initial-reconnect" description:"If true, will apply a randomized staggering between 0s and 30s when reconnecting to persistent peers on startup. The first 10 reconnections will be attempted instantly, regardless of the flag's value"`

	MaxOutgoingCltvExpiry uint32 `long:"max-cltv-expiry" description:"The maximum number of blocks funds could be locked up for when forwarding payments."`

	MaxChannelFeeAllocation float64 `long:"max-channel-fee-allocation" description:"The maximum percentage of total funds that can be allocated to a channel's commitment fee. This only applies for the initiator of the channel. Valid values are within [0.1, 1]."`

	MaxCommitFeeRateAnchors uint64 `long:"max-commit-fee-rate-anchors" description:"The maximum fee rate in sat/vbyte that will be used for commitments of channels of the anchors type. Must be large enough to ensure transaction propagation"`

	DryRunMigration bool `long:"dry-run-migration" description:"If true, lnd will abort committing a migration if it would otherwise have been successful. This leaves the database unmodified, and still compatible with the previously active version of lnd."`

	net tor.Net

	EnableUpfrontShutdown bool `long:"enable-upfront-shutdown" description:"If true, option upfront shutdown script will be enabled. If peers that we open channels with support this feature, we will automatically set the script to which cooperative closes should be paid out to on channel open. This offers the partial protection of a channel peer disconnecting from us if cooperative close is attempted with a different script."`

	AcceptKeySend bool `long:"accept-keysend" description:"If true, spontaneous payments through keysend will be accepted. [experimental]"`

	AcceptAMP bool `long:"accept-amp" description:"If true, spontaneous payments via AMP will be accepted."`

	KeysendHoldTime time.Duration `long:"keysend-hold-time" description:"If non-zero, keysend payments are accepted but not immediately settled. If the payment isn't settled manually after the specified time, it is canceled automatically. [experimental]"`

	GcCanceledInvoicesOnStartup bool `long:"gc-canceled-invoices-on-startup" description:"If true, we'll attempt to garbage collect canceled invoices upon start."`

	GcCanceledInvoicesOnTheFly bool `long:"gc-canceled-invoices-on-the-fly" description:"If true, we'll delete newly canceled invoices on the fly."`

	DustThreshold uint64 `long:"dust-threshold" description:"DEPRECATED: Sets the max fee exposure in satoshis for a channel after which HTLC's will be failed." hidden:"true"`

	MaxFeeExposure uint64 `long:"channel-max-fee-exposure" description:" Limits the maximum fee exposure in satoshis of a channel. This value is enforced for all channels and is independent of the channel initiator."`

	Fee *lncfg.Fee `group:"fee" namespace:"fee"`

	Invoices *lncfg.Invoices `group:"invoices" namespace:"invoices"`

	Routing *lncfg.Routing `group:"routing" namespace:"routing"`

	Gossip *lncfg.Gossip `group:"gossip" namespace:"gossip"`

	Workers *lncfg.Workers `group:"workers" namespace:"workers"`

	Caches *lncfg.Caches `group:"caches" namespace:"caches"`

	Prometheus lncfg.Prometheus `group:"prometheus" namespace:"prometheus"`

	WtClient *lncfg.WtClient `group:"wtclient" namespace:"wtclient"`

	Watchtower *lncfg.Watchtower `group:"watchtower" namespace:"watchtower"`

	ProtocolOptions *lncfg.ProtocolOptions `group:"protocol" namespace:"protocol"`

	AllowCircularRoute bool `long:"allow-circular-route" description:"If true, our node will allow htlc forwards that arrive and depart on the same channel."`

	HealthChecks *lncfg.HealthCheckConfig `group:"healthcheck" namespace:"healthcheck"`

	DB *lncfg.DB `group:"db" namespace:"db"`

	Cluster *lncfg.Cluster `group:"cluster" namespace:"cluster"`

	RPCMiddleware *lncfg.RPCMiddleware `group:"rpcmiddleware" namespace:"rpcmiddleware"`

	RemoteSigner *lncfg.RemoteSigner `group:"remotesigner" namespace:"remotesigner"`

	Sweeper *lncfg.Sweeper `group:"sweeper" namespace:"sweeper"`

	Htlcswitch *lncfg.Htlcswitch `group:"htlcswitch" namespace:"htlcswitch"`

	GRPC *GRPCConfig `group:"grpc" namespace:"grpc"`

	// SubLogMgr is the root logger that all the daemon's subloggers are
	// hooked up to.
	SubLogMgr  *build.SubLoggerManager
	LogRotator *build.RotatingLogWriter
	LogConfig  *build.LogConfig `group:"logging" namespace:"logging"`

	// networkDir is the path to the directory of the currently active
	// network. This path will hold the files related to each different
	// network.
	networkDir string

	// ActiveNetParams contains parameters of the target chain.
	ActiveNetParams chainreg.BitcoinNetParams

	// Estimator is used to estimate routing probabilities.
	Estimator routing.Estimator

	// Dev specifies configs used for integration tests, which is always
	// empty if not built with `integration` flag.
	Dev *lncfg.DevConfig `group:"dev" namespace:"dev"`

	// HTTPHeaderTimeout is the maximum duration that the server will wait
	// before timing out reading the headers of an HTTP request.
	HTTPHeaderTimeout time.Duration `long:"http-header-timeout" description:"The maximum duration that the server will wait before timing out reading the headers of an HTTP request."`

	// NumRestrictedSlots is the max number of incoming connections allowed
	// in the server. Outbound connections are not restricted.
	NumRestrictedSlots uint64 `long:"num-restricted-slots" description:"The max number of incoming connections allowed in the server. Outbound connections are not restricted."`

	// NoDisconnectOnPongFailure controls if we'll disconnect if a peer
	// doesn't respond to a pong in time.
	NoDisconnectOnPongFailure bool `long:"no-disconnect-on-pong-failure" description:"If true, a peer will *not* be disconnected if a pong is not received in time or is mismatched. Defaults to false, meaning peers *will* be disconnected on pong failure."`
}

// GRPCConfig holds the configuration options for the gRPC server.
// See https://github.com/grpc/grpc-go/blob/v1.41.0/keepalive/keepalive.go#L50
// for more details. Any value of 0 means we use the gRPC internal default
// values.
//
//nolint:ll
type GRPCConfig struct {
	// ServerPingTime is a duration for the amount of time of no activity
	// after which the server pings the client to see if the transport is
	// still alive. If set below 1s, a minimum value of 1s will be used
	// instead.
	ServerPingTime time.Duration `long:"server-ping-time" description:"How long the server waits on a gRPC stream with no activity before pinging the client."`

	// ServerPingTimeout is the duration the server waits after having
	// pinged for keepalive check, and if no activity is seen even after
	// that the connection is closed.
	ServerPingTimeout time.Duration `long:"server-ping-timeout" description:"How long the server waits for the response from the client for the keepalive ping response."`

	// ClientPingMinWait is the minimum amount of time a client should wait
	// before sending a keepalive ping.
	ClientPingMinWait time.Duration `long:"client-ping-min-wait" description:"The minimum amount of time the client should wait before sending a keepalive ping."`

	// ClientAllowPingWithoutStream specifies whether pings from the client
	// are allowed even if there are no active gRPC streams. This might be
	// useful to keep the underlying HTTP/2 connection open for future
	// requests.
	ClientAllowPingWithoutStream bool `long:"client-allow-ping-without-stream" description:"If true, the server allows keepalive pings from the client even when there are no active gRPC streams. This might be useful to keep the underlying HTTP/2 connection open for future requests."`
}

// DefaultConfig returns all default values for the Config struct.
//
//nolint:ll
func DefaultConfig() Config {
	return Config{
		LndDir:            DefaultLndDir,
		ConfigFile:        DefaultConfigFile,
		DataDir:           defaultDataDir,
		DebugLevel:        defaultLogLevel,
		TLSCertPath:       defaultTLSCertPath,
		TLSKeyPath:        defaultTLSKeyPath,
		TLSCertDuration:   defaultTLSCertDuration,
		LetsEncryptDir:    defaultLetsEncryptDir,
		LetsEncryptListen: defaultLetsEncryptListen,
		LogDir:            defaultLogDir,
		AcceptorTimeout:   defaultAcceptorTimeout,
		WSPingInterval:    lnrpc.DefaultPingInterval,
		WSPongWait:        lnrpc.DefaultPongWait,
		Bitcoin: &lncfg.Chain{
			MinHTLCIn:     chainreg.DefaultBitcoinMinHTLCInMSat,
			MinHTLCOut:    chainreg.DefaultBitcoinMinHTLCOutMSat,
			BaseFee:       chainreg.DefaultBitcoinBaseFeeMSat,
			FeeRate:       chainreg.DefaultBitcoinFeeRate,
			TimeLockDelta: chainreg.DefaultBitcoinTimeLockDelta,
			MaxLocalDelay: defaultMaxLocalCSVDelay,
			Node:          btcdBackendName,
		},
		BtcdMode: &lncfg.Btcd{
			Dir:     defaultBtcdDir,
			RPCHost: defaultRPCHost,
			RPCCert: defaultBtcdRPCCertFile,
		},
		BitcoindMode: &lncfg.Bitcoind{
			Dir:                defaultBitcoindDir,
			RPCHost:            defaultRPCHost,
			EstimateMode:       defaultBitcoindEstimateMode,
			PrunedNodeMaxPeers: defaultPrunedNodeMaxPeers,
			ZMQReadDeadline:    defaultZMQReadDeadline,
		},
		NeutrinoMode: &lncfg.Neutrino{
			UserAgentName:    neutrino.UserAgentName,
			UserAgentVersion: neutrino.UserAgentVersion,
			MaxPeers:         defaultNeutrinoMaxPeers,
		},
		BlockCacheSize:     defaultBlockCacheSize,
		MaxPendingChannels: lncfg.DefaultMaxPendingChannels,
		NoSeedBackup:       defaultNoSeedBackup,
		MinBackoff:         defaultMinBackoff,
		MaxBackoff:         defaultMaxBackoff,
		ConnectionTimeout:  tor.DefaultConnTimeout,

		Fee: &lncfg.Fee{
			MinUpdateTimeout: lncfg.DefaultMinUpdateTimeout,
			MaxUpdateTimeout: lncfg.DefaultMaxUpdateTimeout,
		},

		SubRPCServers: &subRPCServerConfigs{
			SignRPC:   &signrpc.Config{},
			RouterRPC: routerrpc.DefaultConfig(),
			PeersRPC:  &peersrpc.Config{},
		},
		Autopilot: &lncfg.AutoPilot{
			MaxChannels:    5,
			Allocation:     0.6,
			MinChannelSize: int64(funding.MinChanFundingSize),
			MaxChannelSize: int64(MaxFundingAmount),
			MinConfs:       1,
			ConfTarget:     autopilot.DefaultConfTarget,
			Heuristic: map[string]float64{
				"top_centrality": 1.0,
			},
		},
		PaymentsExpirationGracePeriod: defaultPaymentsExpirationGracePeriod,
		TrickleDelay:                  defaultTrickleDelay,
		ChanStatusSampleInterval:      defaultChanStatusSampleInterval,
		ChanEnableTimeout:             defaultChanEnableTimeout,
		ChanDisableTimeout:            defaultChanDisableTimeout,
		HeightHintCacheQueryDisable:   defaultHeightHintCacheQueryDisable,
		Alias:                         defaultAlias,
		Color:                         defaultColor,
		MinChanSize:                   int64(funding.MinChanFundingSize),
		MaxChanSize:                   int64(0),
		CoopCloseTargetConfs:          defaultCoopCloseTargetConfs,
		DefaultRemoteMaxHtlcs:         defaultRemoteMaxHtlcs,
		NumGraphSyncPeers:             defaultMinPeers,
		HistoricalSyncInterval:        discovery.DefaultHistoricalSyncInterval,
		Tor: &lncfg.Tor{
			SOCKS:   defaultTorSOCKS,
			DNS:     defaultTorDNS,
			Control: defaultTorControl,
		},
		net: &tor.ClearNet{},
		Workers: &lncfg.Workers{
			Read:  lncfg.DefaultReadWorkers,
			Write: lncfg.DefaultWriteWorkers,
			Sig:   lncfg.DefaultSigWorkers,
		},
		Caches: &lncfg.Caches{
			RejectCacheSize:  channeldb.DefaultRejectCacheSize,
			ChannelCacheSize: channeldb.DefaultChannelCacheSize,
		},
		Prometheus: lncfg.DefaultPrometheus(),
		Watchtower: lncfg.DefaultWatchtowerCfg(defaultTowerDir),
		HealthChecks: &lncfg.HealthCheckConfig{
			ChainCheck: &lncfg.CheckConfig{
				Interval: defaultChainInterval,
				Timeout:  defaultChainTimeout,
				Attempts: defaultChainAttempts,
				Backoff:  defaultChainBackoff,
			},
			DiskCheck: &lncfg.DiskCheckConfig{
				RequiredRemaining: defaultRequiredDisk,
				CheckConfig: &lncfg.CheckConfig{
					Interval: defaultDiskInterval,
					Attempts: defaultDiskAttempts,
					Timeout:  defaultDiskTimeout,
					Backoff:  defaultDiskBackoff,
				},
			},
			TLSCheck: &lncfg.CheckConfig{
				Interval: defaultTLSInterval,
				Timeout:  defaultTLSTimeout,
				Attempts: defaultTLSAttempts,
				Backoff:  defaultTLSBackoff,
			},
			TorConnection: &lncfg.CheckConfig{
				Interval: defaultTCInterval,
				Timeout:  defaultTCTimeout,
				Attempts: defaultTCAttempts,
				Backoff:  defaultTCBackoff,
			},
			RemoteSigner: &lncfg.CheckConfig{
				Interval: defaultRSInterval,
				Timeout:  defaultRSTimeout,
				Attempts: defaultRSAttempts,
				Backoff:  defaultRSBackoff,
			},
			LeaderCheck: &lncfg.CheckConfig{
				Interval: defaultLeaderCheckInterval,
				Timeout:  defaultLeaderCheckTimeout,
				Attempts: defaultLeaderCheckAttempts,
				Backoff:  defaultLeaderCheckBackoff,
			},
		},
		Gossip: &lncfg.Gossip{
			MaxChannelUpdateBurst: discovery.DefaultMaxChannelUpdateBurst,
			ChannelUpdateInterval: discovery.DefaultChannelUpdateInterval,
			SubBatchDelay:         discovery.DefaultSubBatchDelay,
			AnnouncementConf:      discovery.DefaultProofMatureDelta,
			MsgRateBytes:          discovery.DefaultMsgBytesPerSecond,
			MsgBurstBytes:         discovery.DefaultMsgBytesBurst,
		},
		Invoices: &lncfg.Invoices{
			HoldExpiryDelta: lncfg.DefaultHoldInvoiceExpiryDelta,
		},
		Routing: &lncfg.Routing{
			BlindedPaths: lncfg.BlindedPaths{
				MinNumRealHops:           lncfg.DefaultMinNumRealBlindedPathHops,
				NumHops:                  lncfg.DefaultNumBlindedPathHops,
				MaxNumPaths:              lncfg.DefaultMaxNumBlindedPaths,
				PolicyIncreaseMultiplier: lncfg.DefaultBlindedPathPolicyIncreaseMultiplier,
				PolicyDecreaseMultiplier: lncfg.DefaultBlindedPathPolicyDecreaseMultiplier,
			},
		},
		MaxOutgoingCltvExpiry:     htlcswitch.DefaultMaxOutgoingCltvExpiry,
		MaxChannelFeeAllocation:   htlcswitch.DefaultMaxLinkFeeAllocation,
		MaxCommitFeeRateAnchors:   lnwallet.DefaultAnchorsCommitMaxFeeRateSatPerVByte,
		LogRotator:                build.NewRotatingLogWriter(),
		DB:                        lncfg.DefaultDB(),
		Cluster:                   lncfg.DefaultCluster(),
		RPCMiddleware:             lncfg.DefaultRPCMiddleware(),
		ActiveNetParams:           chainreg.BitcoinTestNetParams,
		ChannelCommitInterval:     defaultChannelCommitInterval,
		PendingCommitInterval:     defaultPendingCommitInterval,
		ChannelCommitBatchSize:    defaultChannelCommitBatchSize,
		CoinSelectionStrategy:     defaultCoinSelectionStrategy,
		KeepFailedPaymentAttempts: defaultKeepFailedPaymentAttempts,
		RemoteSigner: &lncfg.RemoteSigner{
			Timeout: lncfg.DefaultRemoteSignerRPCTimeout,
		},
		Sweeper: lncfg.DefaultSweeperConfig(),
		Htlcswitch: &lncfg.Htlcswitch{
			MailboxDeliveryTimeout: htlcswitch.DefaultMailboxDeliveryTimeout,
			QuiescenceTimeout:      lncfg.DefaultQuiescenceTimeout,
		},
		GRPC: &GRPCConfig{
			ServerPingTime:    defaultGrpcServerPingTime,
			ServerPingTimeout: defaultGrpcServerPingTimeout,
			ClientPingMinWait: defaultGrpcClientPingMinWait,
		},
		LogConfig:                 build.DefaultLogConfig(),
		WtClient:                  lncfg.DefaultWtClientCfg(),
		HTTPHeaderTimeout:         DefaultHTTPHeaderTimeout,
		NumRestrictedSlots:        DefaultNumRestrictedSlots,
		NoDisconnectOnPongFailure: defaultNoDisconnectOnPongFailure,
	}
}

// LoadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
//  1. Start with a default config with sane settings
//  2. Pre-parse the command line to check for an alternative config file
//  3. Load configuration file overwriting defaults with any specified options
//  4. Parse CLI options and overwrite/add any specified options
func LoadConfig(interceptor signal.Interceptor) (*Config, error) {
	// Pre-parse the command line options to pick up an alternative config
	// file.
	preCfg := DefaultConfig()
	if _, err := flags.Parse(&preCfg); err != nil {
		return nil, err
	}

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
	if preCfg.ShowVersion {
		fmt.Println(appName, "version", build.Version(),
			"commit="+build.Commit)
		os.Exit(0)
	}

	// If the config file path has not been modified by the user, then we'll
	// use the default config file path. However, if the user has modified
	// their lnddir, then we should assume they intend to use the config
	// file within it.
	configFileDir := CleanAndExpandPath(preCfg.LndDir)
	configFilePath := CleanAndExpandPath(preCfg.ConfigFile)
	switch {
	// User specified --lnddir but no --configfile. Update the config file
	// path to the lnd config directory, but don't require it to exist.
	case configFileDir != DefaultLndDir &&
		configFilePath == DefaultConfigFile:

		configFilePath = filepath.Join(
			configFileDir, lncfg.DefaultConfigFilename,
		)

	// User did specify an explicit --configfile, so we check that it does
	// exist under that path to avoid surprises.
	case configFilePath != DefaultConfigFile:
		if !lnrpc.FileExists(configFilePath) {
			return nil, fmt.Errorf("specified config file does "+
				"not exist in %s", configFilePath)
		}
	}

	// Next, load any additional configuration options from the file.
	var configFileError error
	cfg := preCfg
	fileParser := flags.NewParser(&cfg, flags.Default)
	err := flags.NewIniParser(fileParser).ParseFile(configFilePath)
	if err != nil {
		// If it's a parsing related error, then we'll return
		// immediately, otherwise we can proceed as possibly the config
		// file doesn't exist which is OK.
		if lnutils.ErrorAs[*flags.IniError](err) ||
			lnutils.ErrorAs[*flags.Error](err) {

			return nil, err
		}

		configFileError = err
	}

	// Finally, parse the remaining command line options again to ensure
	// they take precedence.
	flagParser := flags.NewParser(&cfg, flags.Default)
	if _, err := flagParser.Parse(); err != nil {
		return nil, err
	}

	// Make sure everything we just loaded makes sense.
	cleanCfg, err := ValidateConfig(
		cfg, interceptor, fileParser, flagParser,
	)
	var usageErr *lncfg.UsageError
	if errors.As(err, &usageErr) {
		// The logging system might not yet be initialized, so we also
		// write to stderr to make sure the error appears somewhere.
		_, _ = fmt.Fprintln(os.Stderr, usageMessage)
		ltndLog.Warnf("Incorrect usage: %v", usageMessage)

		// The log subsystem might not yet be initialized. But we still
		// try to log the error there since some packaging solutions
		// might only look at the log and not stdout/stderr.
		ltndLog.Warnf("Error validating config: %v", err)

		return nil, err
	}
	if err != nil {
		// The log subsystem might not yet be initialized. But we still
		// try to log the error there since some packaging solutions
		// might only look at the log and not stdout/stderr.
		ltndLog.Warnf("Error validating config: %v", err)

		return nil, err
	}

	// Warn about missing config file only after all other configuration is
	// done. This prevents the warning on help messages and invalid options.
	// Note this should go directly before the return.
	if configFileError != nil {
		ltndLog.Warnf("%v", configFileError)
	}

	// Finally, log warnings for deprecated config options if they are set.
	logWarningsForDeprecation(*cleanCfg)

	return cleanCfg, nil
}

// ValidateConfig check the given configuration to be sane. This makes sure no
// illegal values or combination of values are set. All file system paths are
// normalized. The cleaned up config is returned on success.
func ValidateConfig(cfg Config, interceptor signal.Interceptor, fileParser,
	flagParser *flags.Parser) (*Config, error) {

	// Special show command to list supported subsystems and exit.
	if cfg.DebugLevel == "show" {
		subLogMgr := build.NewSubLoggerManager()

		// Initialize logging at the default logging level.
		SetupLoggers(subLogMgr, interceptor)

		fmt.Println("Supported subsystems",
			subLogMgr.SupportedSubsystems())
		os.Exit(0)
	}

	// If the provided lnd directory is not the default, we'll modify the
	// path to all of the files and directories that will live within it.
	lndDir := CleanAndExpandPath(cfg.LndDir)
	if lndDir != DefaultLndDir {
		cfg.DataDir = filepath.Join(lndDir, defaultDataDirname)
		cfg.LetsEncryptDir = filepath.Join(
			lndDir, defaultLetsEncryptDirname,
		)
		cfg.TLSCertPath = filepath.Join(lndDir, defaultTLSCertFilename)
		cfg.TLSKeyPath = filepath.Join(lndDir, defaultTLSKeyFilename)
		cfg.LogDir = filepath.Join(lndDir, defaultLogDirname)

		// If the watchtower's directory is set to the default, i.e. the
		// user has not requested a different location, we'll move the
		// location to be relative to the specified lnd directory.
		if cfg.Watchtower.TowerDir == defaultTowerDir {
			cfg.Watchtower.TowerDir = filepath.Join(
				cfg.DataDir, defaultTowerSubDirname,
			)
		}
	}

	funcName := "ValidateConfig"
	mkErr := func(format string, args ...interface{}) error {
		return fmt.Errorf(funcName+": "+format, args...)
	}
	makeDirectory := func(dir string) error {
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			// Show a nicer error message if it's because a symlink
			// is linked to a directory that does not exist
			// (probably because it's not mounted).
			if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
				link, lerr := os.Readlink(e.Path)
				if lerr == nil {
					str := "is symlink %s -> %s mounted?"
					err = fmt.Errorf(str, e.Path, link)
				}
			}

			str := "Failed to create lnd directory '%s': %v"
			return mkErr(str, dir, err)
		}

		return nil
	}

	// IsSet returns true if an option has been set in either the config
	// file or by a flag.
	isSet := func(field string) (bool, error) {
		fieldName, ok := reflect.TypeOf(Config{}).FieldByName(field)
		if !ok {
			str := "could not find field %s"
			return false, mkErr(str, field)
		}

		long, ok := fieldName.Tag.Lookup("long")
		if !ok {
			str := "field %s does not have a long tag"
			return false, mkErr(str, field)
		}

		// The user has the option to set the flag in either the config
		// file or as a command line flag. If any is set, we consider it
		// to be set, not applying any precedence rules here (since it
		// is a boolean the default is false anyway which would screw up
		// any precedence rules). Additionally, we need to also support
		// the use case where the config struct is embedded _within_
		// another struct with a prefix (as is the case with
		// lightning-terminal).
		fileOption := fileParser.FindOptionByLongName(long)
		fileOptionNested := fileParser.FindOptionByLongName(
			"lnd." + long,
		)
		flagOption := flagParser.FindOptionByLongName(long)
		flagOptionNested := flagParser.FindOptionByLongName(
			"lnd." + long,
		)

		return (fileOption != nil && fileOption.IsSet()) ||
				(fileOptionNested != nil && fileOptionNested.IsSet()) ||
				(flagOption != nil && flagOption.IsSet()) ||
				(flagOptionNested != nil && flagOptionNested.IsSet()),
			nil
	}

	// As soon as we're done parsing configuration options, ensure all paths
	// to directories and files are cleaned and expanded before attempting
	// to use them later on.
	cfg.DataDir = CleanAndExpandPath(cfg.DataDir)
	cfg.TLSCertPath = CleanAndExpandPath(cfg.TLSCertPath)
	cfg.TLSKeyPath = CleanAndExpandPath(cfg.TLSKeyPath)
	cfg.LetsEncryptDir = CleanAndExpandPath(cfg.LetsEncryptDir)
	cfg.AdminMacPath = CleanAndExpandPath(cfg.AdminMacPath)
	cfg.ReadMacPath = CleanAndExpandPath(cfg.ReadMacPath)
	cfg.InvoiceMacPath = CleanAndExpandPath(cfg.InvoiceMacPath)
	cfg.LogDir = CleanAndExpandPath(cfg.LogDir)
	cfg.BtcdMode.Dir = CleanAndExpandPath(cfg.BtcdMode.Dir)
	cfg.BitcoindMode.Dir = CleanAndExpandPath(cfg.BitcoindMode.Dir)
	cfg.BitcoindMode.ConfigPath = CleanAndExpandPath(
		cfg.BitcoindMode.ConfigPath,
	)
	cfg.BitcoindMode.RPCCookie = CleanAndExpandPath(cfg.BitcoindMode.RPCCookie)
	cfg.Tor.PrivateKeyPath = CleanAndExpandPath(cfg.Tor.PrivateKeyPath)
	cfg.Tor.WatchtowerKeyPath = CleanAndExpandPath(cfg.Tor.WatchtowerKeyPath)
	cfg.Watchtower.TowerDir = CleanAndExpandPath(cfg.Watchtower.TowerDir)
	cfg.BackupFilePath = CleanAndExpandPath(cfg.BackupFilePath)
	cfg.WalletUnlockPasswordFile = CleanAndExpandPath(
		cfg.WalletUnlockPasswordFile,
	)

	// Ensure that the user didn't attempt to specify negative values for
	// any of the autopilot params.
	if cfg.Autopilot.MaxChannels < 0 {
		str := "autopilot.maxchannels must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.Allocation < 0 {
		str := "autopilot.allocation must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.MinChannelSize < 0 {
		str := "autopilot.minchansize must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.MaxChannelSize < 0 {
		str := "autopilot.maxchansize must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.MinConfs < 0 {
		str := "autopilot.minconfs must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.ConfTarget < 1 {
		str := "autopilot.conftarget must be positive"

		return nil, mkErr(str)
	}

	// Ensure that the specified values for the min and max channel size
	// are within the bounds of the normal chan size constraints.
	if cfg.Autopilot.MinChannelSize < int64(funding.MinChanFundingSize) {
		cfg.Autopilot.MinChannelSize = int64(funding.MinChanFundingSize)
	}
	if cfg.Autopilot.MaxChannelSize > int64(MaxFundingAmount) {
		cfg.Autopilot.MaxChannelSize = int64(MaxFundingAmount)
	}

	if _, err := validateAtplCfg(cfg.Autopilot); err != nil {
		return nil, mkErr("error validating autopilot: %v", err)
	}

	// Ensure that --maxchansize is properly handled when set by user.
	// For non-Wumbo channels this limit remains 16777215 satoshis by default
	// as specified in BOLT-02. For wumbo channels this limit is 1,000,000,000.
	// satoshis (10 BTC). Always enforce --maxchansize explicitly set by user.
	// If unset (marked by 0 value), then enforce proper default.
	if cfg.MaxChanSize == 0 {
		if cfg.ProtocolOptions.Wumbo() {
			cfg.MaxChanSize = int64(funding.MaxBtcFundingAmountWumbo)
		} else {
			cfg.MaxChanSize = int64(funding.MaxBtcFundingAmount)
		}
	}

	// Ensure that the user specified values for the min and max channel
	// size make sense.
	if cfg.MaxChanSize < cfg.MinChanSize {
		return nil, mkErr("invalid channel size parameters: "+
			"max channel size %v, must be no less than min chan "+
			"size %v", cfg.MaxChanSize, cfg.MinChanSize,
		)
	}

	// Don't allow superfluous --maxchansize greater than
	// BOLT 02 soft-limit for non-wumbo channel
	if !cfg.ProtocolOptions.Wumbo() &&
		cfg.MaxChanSize > int64(MaxFundingAmount) {

		return nil, mkErr("invalid channel size parameters: "+
			"maximum channel size %v is greater than maximum "+
			"non-wumbo channel size %v", cfg.MaxChanSize,
			MaxFundingAmount,
		)
	}

	// Ensure that the amount data for revoked commitment transactions is
	// stored if the watchtower client is active.
	if cfg.DB.NoRevLogAmtData && cfg.WtClient.Active {
		return nil, mkErr("revocation log amount data must be stored " +
			"if the watchtower client is active")
	}

	// Ensure a valid max channel fee allocation was set.
	if cfg.MaxChannelFeeAllocation <= 0 || cfg.MaxChannelFeeAllocation > 1 {
		return nil, mkErr("invalid max channel fee allocation: %v, "+
			"must be within (0, 1]", cfg.MaxChannelFeeAllocation)
	}

	if cfg.MaxCommitFeeRateAnchors < 1 {
		return nil, mkErr("invalid max commit fee rate anchors: %v, "+
			"must be at least 1 sat/vByte",
			cfg.MaxCommitFeeRateAnchors)
	}

	// Validate the Tor config parameters.
	socks, err := lncfg.ParseAddressString(
		cfg.Tor.SOCKS, strconv.Itoa(defaultTorSOCKSPort),
		cfg.net.ResolveTCPAddr,
	)
	if err != nil {
		return nil, err
	}
	cfg.Tor.SOCKS = socks.String()

	// We'll only attempt to normalize and resolve the DNS host if it hasn't
	// changed, as it doesn't need to be done for the default.
	if cfg.Tor.DNS != defaultTorDNS {
		dns, err := lncfg.ParseAddressString(
			cfg.Tor.DNS, strconv.Itoa(defaultTorDNSPort),
			cfg.net.ResolveTCPAddr,
		)
		if err != nil {
			return nil, mkErr("error parsing tor dns: %v", err)
		}
		cfg.Tor.DNS = dns.String()
	}

	control, err := lncfg.ParseAddressString(
		cfg.Tor.Control, strconv.Itoa(defaultTorControlPort),
		cfg.net.ResolveTCPAddr,
	)
	if err != nil {
		return nil, mkErr("error parsing tor control address: %v", err)
	}
	cfg.Tor.Control = control.String()

	// Ensure that tor socks host:port is not equal to tor control
	// host:port. This would lead to lnd not starting up properly.
	if cfg.Tor.SOCKS == cfg.Tor.Control {
		str := "tor.socks and tor.control can not us the same host:port"

		return nil, mkErr(str)
	}

	switch {
	case cfg.Tor.V2 && cfg.Tor.V3:
		return nil, mkErr("either tor.v2 or tor.v3 can be set, " +
			"but not both")
	case cfg.DisableListen && (cfg.Tor.V2 || cfg.Tor.V3):
		return nil, mkErr("listening must be enabled when enabling " +
			"inbound connections over Tor")
	}

	if cfg.Tor.PrivateKeyPath == "" {
		switch {
		case cfg.Tor.V2:
			cfg.Tor.PrivateKeyPath = filepath.Join(
				lndDir, defaultTorV2PrivateKeyFilename,
			)
		case cfg.Tor.V3:
			cfg.Tor.PrivateKeyPath = filepath.Join(
				lndDir, defaultTorV3PrivateKeyFilename,
			)
		}
	}

	if cfg.Tor.WatchtowerKeyPath == "" {
		switch {
		case cfg.Tor.V2:
			cfg.Tor.WatchtowerKeyPath = filepath.Join(
				cfg.Watchtower.TowerDir,
				defaultTorV2PrivateKeyFilename,
			)
		case cfg.Tor.V3:
			cfg.Tor.WatchtowerKeyPath = filepath.Join(
				cfg.Watchtower.TowerDir,
				defaultTorV3PrivateKeyFilename,
			)
		}
	}

	// Set up the network-related functions that will be used throughout
	// the daemon. We use the standard Go "net" package functions by
	// default. If we should be proxying all traffic through Tor, then
	// we'll use the Tor proxy specific functions in order to avoid leaking
	// our real information.
	if cfg.Tor.Active {
		cfg.net = &tor.ProxyNet{
			SOCKS:                       cfg.Tor.SOCKS,
			DNS:                         cfg.Tor.DNS,
			StreamIsolation:             cfg.Tor.StreamIsolation,
			SkipProxyForClearNetTargets: cfg.Tor.SkipProxyForClearNetTargets,
		}
	}

	if cfg.DisableListen && cfg.NAT {
		return nil, mkErr("NAT traversal cannot be used when " +
			"listening is disabled")
	}
	if cfg.NAT && len(cfg.ExternalHosts) != 0 {
		return nil, mkErr("NAT support and externalhosts are " +
			"mutually exclusive, only one should be selected")
	}

	// Multiple networks can't be selected simultaneously.  Count
	// number of network flags passed; assign active network params
	// while we're at it.
	numNets := 0
	if cfg.Bitcoin.MainNet {
		numNets++
		cfg.ActiveNetParams = chainreg.BitcoinMainNetParams
	}
	if cfg.Bitcoin.TestNet3 {
		numNets++
		cfg.ActiveNetParams = chainreg.BitcoinTestNetParams
	}
	if cfg.Bitcoin.TestNet4 {
		numNets++
		cfg.ActiveNetParams = chainreg.BitcoinTestNet4Params
	}
	if cfg.Bitcoin.RegTest {
		numNets++
		cfg.ActiveNetParams = chainreg.BitcoinRegTestNetParams
	}
	if cfg.Bitcoin.SimNet {
		numNets++
		cfg.ActiveNetParams = chainreg.BitcoinSimNetParams

		// For simnet, the btcsuite chain params uses a
		// cointype of 115. However, we override this in
		// chainreg/chainparams.go, but the raw ChainParam
		// field is used elsewhere. To ensure everything is
		// consistent, we'll also override the cointype within
		// the raw params.
		targetCoinType := chainreg.BitcoinSigNetParams.CoinType
		cfg.ActiveNetParams.Params.HDCoinType = targetCoinType
	}
	if cfg.Bitcoin.SigNet {
		numNets++
		cfg.ActiveNetParams = chainreg.BitcoinSigNetParams

		// Let the user overwrite the default signet parameters.
		// The challenge defines the actual signet network to
		// join and the seed nodes are needed for network
		// discovery.
		sigNetChallenge := chaincfg.DefaultSignetChallenge
		sigNetSeeds := chaincfg.DefaultSignetDNSSeeds
		if cfg.Bitcoin.SigNetChallenge != "" {
			challenge, err := hex.DecodeString(
				cfg.Bitcoin.SigNetChallenge,
			)
			if err != nil {
				return nil, mkErr("Invalid "+
					"signet challenge, hex decode "+
					"failed: %v", err)
			}
			sigNetChallenge = challenge
		}

		if len(cfg.Bitcoin.SigNetSeedNode) > 0 {
			sigNetSeeds = make([]chaincfg.DNSSeed, len(
				cfg.Bitcoin.SigNetSeedNode,
			))
			for idx, seed := range cfg.Bitcoin.SigNetSeedNode {
				sigNetSeeds[idx] = chaincfg.DNSSeed{
					Host:         seed,
					HasFiltering: false,
				}
			}
		}

		chainParams := chaincfg.CustomSignetParams(
			sigNetChallenge, sigNetSeeds,
		)
		cfg.ActiveNetParams.Params = &chainParams
	}
	if numNets > 1 {
		str := "The mainnet, testnet, testnet4, regtest, simnet and " +
			"signet params can't be used together -- choose one " +
			"of the five"

		return nil, mkErr(str)
	}

	// The target network must be provided, otherwise, we won't
	// know how to initialize the daemon.
	if numNets == 0 {
		str := "either --bitcoin.mainnet, or --bitcoin.testnet, " +
			"--bitcoin.testnet4, --bitcoin.simnet, " +
			"--bitcoin.regtest or --bitcoin.signet must be " +
			"specified"

		return nil, mkErr(str)
	}

	err = cfg.Bitcoin.Validate(minTimeLockDelta, funding.MinBtcRemoteDelay)
	if err != nil {
		return nil, mkErr("error validating bitcoin params: %v", err)
	}

	switch cfg.Bitcoin.Node {
	case btcdBackendName:
		err := parseRPCParams(
			cfg.Bitcoin, cfg.BtcdMode, cfg.ActiveNetParams,
		)
		if err != nil {
			return nil, mkErr("unable to load RPC "+
				"credentials for btcd: %v", err)
		}
	case bitcoindBackendName:
		if cfg.Bitcoin.SimNet {
			return nil, mkErr("bitcoind does not " +
				"support simnet")
		}

		err := parseRPCParams(
			cfg.Bitcoin, cfg.BitcoindMode, cfg.ActiveNetParams,
		)
		if err != nil {
			return nil, mkErr("unable to load RPC "+
				"credentials for bitcoind: %v", err)
		}
	case neutrinoBackendName:
		// No need to get RPC parameters.

	case "nochainbackend":
		// Nothing to configure, we're running without any chain
		// backend whatsoever (pure signing mode).

	default:
		str := "only btcd, bitcoind, and neutrino mode " +
			"supported for bitcoin at this time"

		return nil, mkErr(str)
	}

	cfg.Bitcoin.ChainDir = filepath.Join(
		cfg.DataDir, defaultChainSubDirname, BitcoinChainName,
	)

	// Ensure that the user didn't attempt to specify negative values for
	// any of the autopilot params.
	if cfg.Autopilot.MaxChannels < 0 {
		str := "autopilot.maxchannels must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.Allocation < 0 {
		str := "autopilot.allocation must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.MinChannelSize < 0 {
		str := "autopilot.minchansize must be non-negative"

		return nil, mkErr(str)
	}
	if cfg.Autopilot.MaxChannelSize < 0 {
		str := "autopilot.maxchansize must be non-negative"

		return nil, mkErr(str)
	}

	// Ensure that the specified values for the min and max channel size
	// don't are within the bounds of the normal chan size constraints.
	if cfg.Autopilot.MinChannelSize < int64(funding.MinChanFundingSize) {
		cfg.Autopilot.MinChannelSize = int64(funding.MinChanFundingSize)
	}
	if cfg.Autopilot.MaxChannelSize > int64(MaxFundingAmount) {
		cfg.Autopilot.MaxChannelSize = int64(MaxFundingAmount)
	}

	// We'll now construct the network directory which will be where we
	// store all the data specific to this chain/network.
	cfg.networkDir = filepath.Join(
		cfg.DataDir, defaultChainSubDirname, BitcoinChainName,
		lncfg.NormalizeNetwork(cfg.ActiveNetParams.Name),
	)

	// If a custom macaroon directory wasn't specified and the data
	// directory has changed from the default path, then we'll also update
	// the path for the macaroons to be generated.
	if cfg.AdminMacPath == "" {
		cfg.AdminMacPath = filepath.Join(
			cfg.networkDir, defaultAdminMacFilename,
		)
	}
	if cfg.ReadMacPath == "" {
		cfg.ReadMacPath = filepath.Join(
			cfg.networkDir, defaultReadMacFilename,
		)
	}
	if cfg.InvoiceMacPath == "" {
		cfg.InvoiceMacPath = filepath.Join(
			cfg.networkDir, defaultInvoiceMacFilename,
		)
	}

	towerDir := filepath.Join(
		cfg.Watchtower.TowerDir, BitcoinChainName,
		lncfg.NormalizeNetwork(cfg.ActiveNetParams.Name),
	)

	// Create the lnd directory and all other sub-directories if they don't
	// already exist. This makes sure that directory trees are also created
	// for files that point to outside the lnddir.
	dirs := []string{
		lndDir, cfg.DataDir, cfg.networkDir,
		cfg.LetsEncryptDir, towerDir, cfg.graphDatabaseDir(),
		filepath.Dir(cfg.TLSCertPath), filepath.Dir(cfg.TLSKeyPath),
		filepath.Dir(cfg.AdminMacPath), filepath.Dir(cfg.ReadMacPath),
		filepath.Dir(cfg.InvoiceMacPath),
		filepath.Dir(cfg.Tor.PrivateKeyPath),
		filepath.Dir(cfg.Tor.WatchtowerKeyPath),
	}
	for _, dir := range dirs {
		if err := makeDirectory(dir); err != nil {
			return nil, err
		}
	}

	// Similarly, if a custom back up file path wasn't specified, then
	// we'll update the file location to match our set network directory.
	if cfg.BackupFilePath == "" {
		cfg.BackupFilePath = filepath.Join(
			cfg.networkDir, chanbackup.DefaultBackupFileName,
		)
	}

	// Append the network type to the log directory so it is "namespaced"
	// per network in the same fashion as the data directory.
	cfg.LogDir = filepath.Join(
		cfg.LogDir, BitcoinChainName,
		lncfg.NormalizeNetwork(cfg.ActiveNetParams.Name),
	)

	if err := cfg.LogConfig.Validate(); err != nil {
		return nil, mkErr("error validating logging config: %w", err)
	}

	// If a sub-log manager was not already created, then we'll create one
	// now using the default log handlers.
	if cfg.SubLogMgr == nil {
		cfg.SubLogMgr = build.NewSubLoggerManager(
			build.NewDefaultLogHandlers(
				cfg.LogConfig, cfg.LogRotator,
			)...,
		)
	}

	// Initialize logging at the default logging level.
	SetupLoggers(cfg.SubLogMgr, interceptor)

	if cfg.MaxLogFiles != 0 {
		if cfg.LogConfig.File.MaxLogFiles !=
			build.DefaultMaxLogFiles {

			return nil, mkErr("cannot set both maxlogfiles and "+
				"logging.file.max-files", err)
		}

		cfg.LogConfig.File.MaxLogFiles = cfg.MaxLogFiles
	}
	if cfg.MaxLogFileSize != 0 {
		if cfg.LogConfig.File.MaxLogFileSize !=
			build.DefaultMaxLogFileSize {

			return nil, mkErr("cannot set both maxlogfilesize and "+
				"logging.file.max-file-size", err)
		}

		cfg.LogConfig.File.MaxLogFileSize = cfg.MaxLogFileSize
	}

	err = cfg.LogRotator.InitLogRotator(
		cfg.LogConfig.File,
		filepath.Join(cfg.LogDir, defaultLogFilename),
	)
	if err != nil {
		str := "log rotation setup failed: %v"
		return nil, mkErr(str, err)
	}

	// Parse, validate, and set debug log level(s).
	err = build.ParseAndSetDebugLevels(cfg.DebugLevel, cfg.SubLogMgr)
	if err != nil {
		str := "error parsing debug level: %v"
		return nil, &lncfg.UsageError{Err: mkErr(str, err)}
	}

	// At least one RPCListener is required. So listen on localhost per
	// default.
	if len(cfg.RawRPCListeners) == 0 {
		addr := fmt.Sprintf("localhost:%d", defaultRPCPort)
		cfg.RawRPCListeners = append(cfg.RawRPCListeners, addr)
	}

	// Listen on localhost if no REST listeners were specified.
	if len(cfg.RawRESTListeners) == 0 {
		addr := fmt.Sprintf("localhost:%d", defaultRESTPort)
		cfg.RawRESTListeners = append(cfg.RawRESTListeners, addr)
	}

	// Listen on the default interface/port if no listeners were specified.
	// An empty address string means default interface/address, which on
	// most unix systems is the same as 0.0.0.0. If Tor is active, we
	// default to only listening on localhost for hidden service
	// connections.
	if len(cfg.RawListeners) == 0 {
		addr := fmt.Sprintf(":%d", defaultPeerPort)
		if cfg.Tor.Active && !cfg.Tor.SkipProxyForClearNetTargets {
			addr = fmt.Sprintf("localhost:%d", defaultPeerPort)
		}
		cfg.RawListeners = append(cfg.RawListeners, addr)
	}

	// Add default port to all RPC listener addresses if needed and remove
	// duplicate addresses.
	cfg.RPCListeners, err = lncfg.NormalizeAddresses(
		cfg.RawRPCListeners, strconv.Itoa(defaultRPCPort),
		cfg.net.ResolveTCPAddr,
	)
	if err != nil {
		return nil, mkErr("error normalizing RPC listen addrs: %v", err)
	}

	// Add default port to all REST listener addresses if needed and remove
	// duplicate addresses.
	cfg.RESTListeners, err = lncfg.NormalizeAddresses(
		cfg.RawRESTListeners, strconv.Itoa(defaultRESTPort),
		cfg.net.ResolveTCPAddr,
	)
	if err != nil {
		return nil, mkErr("error normalizing REST listen addrs: %v", err)
	}

	switch {
	// The no seed backup and auto unlock are mutually exclusive.
	case cfg.NoSeedBackup && cfg.WalletUnlockPasswordFile != "":
		return nil, mkErr("cannot set noseedbackup and " +
			"wallet-unlock-password-file at the same time")

	// The "allow-create" flag cannot be set without the auto unlock file.
	case cfg.WalletUnlockAllowCreate && cfg.WalletUnlockPasswordFile == "":
		return nil, mkErr("cannot set wallet-unlock-allow-create " +
			"without wallet-unlock-password-file")

	// If a password file was specified, we need it to exist.
	case cfg.WalletUnlockPasswordFile != "" &&
		!lnrpc.FileExists(cfg.WalletUnlockPasswordFile):

		return nil, mkErr("wallet unlock password file %s does "+
			"not exist", cfg.WalletUnlockPasswordFile)
	}

	// For each of the RPC listeners (REST+gRPC), we'll ensure that users
	// have specified a safe combo for authentication. If not, we'll bail
	// out with an error. Since we don't allow disabling TLS for gRPC
	// connections we pass in tlsActive=true.
	err = lncfg.EnforceSafeAuthentication(
		cfg.RPCListeners, !cfg.NoMacaroons, true,
	)
	if err != nil {
		return nil, mkErr("error enforcing safe authentication on "+
			"RPC ports: %v", err)
	}

	if cfg.DisableRest {
		ltndLog.Infof("REST API is disabled!")
		cfg.RESTListeners = nil
	} else {
		err = lncfg.EnforceSafeAuthentication(
			cfg.RESTListeners, !cfg.NoMacaroons, !cfg.DisableRestTLS,
		)
		if err != nil {
			return nil, mkErr("error enforcing safe "+
				"authentication on REST ports: %v", err)
		}
	}

	// Remove the listening addresses specified if listening is disabled.
	if cfg.DisableListen {
		ltndLog.Infof("Listening on the p2p interface is disabled!")
		cfg.Listeners = nil
		cfg.ExternalIPs = nil
	} else {

		// Add default port to all listener addresses if needed and remove
		// duplicate addresses.
		cfg.Listeners, err = lncfg.NormalizeAddresses(
			cfg.RawListeners, strconv.Itoa(defaultPeerPort),
			cfg.net.ResolveTCPAddr,
		)
		if err != nil {
			return nil, mkErr("error normalizing p2p listen "+
				"addrs: %v", err)
		}

		// Add default port to all external IP addresses if needed and remove
		// duplicate addresses.
		cfg.ExternalIPs, err = lncfg.NormalizeAddresses(
			cfg.RawExternalIPs, strconv.Itoa(defaultPeerPort),
			cfg.net.ResolveTCPAddr,
		)
		if err != nil {
			return nil, err
		}

		// For the p2p port it makes no sense to listen to an Unix socket.
		// Also, we would need to refactor the brontide listener to support
		// that.
		for _, p2pListener := range cfg.Listeners {
			if lncfg.IsUnix(p2pListener) {
				return nil, mkErr("unix socket addresses "+
					"cannot be used for the p2p "+
					"connection listener: %s", p2pListener)
			}
		}
	}

	// Ensure that the specified minimum backoff is below or equal to the
	// maximum backoff.
	if cfg.MinBackoff > cfg.MaxBackoff {
		return nil, mkErr("maxbackoff must be greater than minbackoff")
	}

	// Newer versions of lnd added a new sub-config for bolt-specific
	// parameters. However, we want to also allow existing users to use the
	// value on the top-level config. If the outer config value is set,
	// then we'll use that directly.
	flagSet, err := isSet("SyncFreelist")
	if err != nil {
		return nil, mkErr("error parsing freelist sync flag: %v", err)
	}
	if flagSet {
		cfg.DB.Bolt.NoFreelistSync = !cfg.SyncFreelist
	}

	// Parse any extra sqlite pragma options that may have been provided
	// to determine if they override any of the defaults that we will
	// otherwise add.
	var (
		defaultSynchronous = true
		defaultAutoVacuum  = true
		defaultFullfsync   = true
	)
	for _, option := range cfg.DB.Sqlite.PragmaOptions {
		switch {
		case strings.HasPrefix(option, "synchronous="):
			defaultSynchronous = false

		case strings.HasPrefix(option, "auto_vacuum="):
			defaultAutoVacuum = false

		case strings.HasPrefix(option, "fullfsync="):
			defaultFullfsync = false

		default:
		}
	}

	if defaultSynchronous {
		cfg.DB.Sqlite.PragmaOptions = append(
			cfg.DB.Sqlite.PragmaOptions, "synchronous=full",
		)
	}

	if defaultAutoVacuum {
		cfg.DB.Sqlite.PragmaOptions = append(
			cfg.DB.Sqlite.PragmaOptions, "auto_vacuum=incremental",
		)
	}

	if defaultFullfsync {
		cfg.DB.Sqlite.PragmaOptions = append(
			cfg.DB.Sqlite.PragmaOptions, "fullfsync=true",
		)
	}

	// Ensure that the user hasn't chosen a remote-max-htlc value greater
	// than the protocol maximum.
	maxRemoteHtlcs := uint16(input.MaxHTLCNumber / 2)
	if cfg.DefaultRemoteMaxHtlcs > maxRemoteHtlcs {
		return nil, mkErr("default-remote-max-htlcs (%v) must be "+
			"less than %v", cfg.DefaultRemoteMaxHtlcs,
			maxRemoteHtlcs)
	}

	// Clamp the ChannelCommitInterval so that commitment updates can still
	// happen in a reasonable timeframe.
	if cfg.ChannelCommitInterval > maxChannelCommitInterval {
		return nil, mkErr("channel-commit-interval (%v) must be less "+
			"than %v", cfg.ChannelCommitInterval,
			maxChannelCommitInterval)
	}

	// Limit PendingCommitInterval so we don't wait too long for the remote
	// party to send back a revoke.
	if cfg.PendingCommitInterval > maxPendingCommitInterval {
		return nil, mkErr("pending-commit-interval (%v) must be less "+
			"than %v", cfg.PendingCommitInterval,
			maxPendingCommitInterval)
	}

	if err := cfg.Gossip.Parse(); err != nil {
		return nil, mkErr("error parsing gossip syncer: %v", err)
	}

	// If the experimental protocol options specify any protocol messages
	// that we want to handle as custom messages, set them now.
	customMsg := cfg.ProtocolOptions.CustomMessageOverrides()

	// We can safely set our custom override values during startup because
	// startup is blocked on config parsing.
	if err := lnwire.SetCustomOverrides(customMsg); err != nil {
		return nil, mkErr("custom-message: %v", err)
	}

	// Map old pprof flags to new pprof group flags.
	//
	// NOTE: This is a temporary measure to ensure compatibility with old
	// flags.
	if cfg.CPUProfile != "" {
		if cfg.Pprof.CPUProfile != "" {
			return nil, mkErr("cpuprofile and pprof.cpuprofile " +
				"are mutually exclusive")
		}
		cfg.Pprof.CPUProfile = cfg.CPUProfile
	}
	if cfg.Profile != "" {
		if cfg.Pprof.Profile != "" {
			return nil, mkErr("profile and pprof.profile " +
				"are mutually exclusive")
		}
		cfg.Pprof.Profile = cfg.Profile
	}
	if cfg.BlockingProfile != 0 {
		if cfg.Pprof.BlockingProfile != 0 {
			return nil, mkErr("blockingprofile and " +
				"pprof.blockingprofile are mutually exclusive")
		}
		cfg.Pprof.BlockingProfile = cfg.BlockingProfile
	}
	if cfg.MutexProfile != 0 {
		if cfg.Pprof.MutexProfile != 0 {
			return nil, mkErr("mutexprofile and " +
				"pprof.mutexprofile are mutually exclusive")
		}
		cfg.Pprof.MutexProfile = cfg.MutexProfile
	}

	// Don't allow both the old dust-threshold and the new
	// channel-max-fee-exposure to be set.
	if cfg.DustThreshold != 0 && cfg.MaxFeeExposure != 0 {
		return nil, mkErr("cannot set both dust-threshold and " +
			"channel-max-fee-exposure")
	}

	switch {
	// Use the old dust-threshold as the max fee exposure if it is set and
	// the new option is not.
	case cfg.DustThreshold != 0:
		cfg.MaxFeeExposure = cfg.DustThreshold

	// Use the default max fee exposure if the new option is not set and
	// the old one is not set either.
	case cfg.MaxFeeExposure == 0:
		cfg.MaxFeeExposure = uint64(
			htlcswitch.DefaultMaxFeeExposure.ToSatoshis(),
		)
	}

	// Validate the subconfigs for workers, caches, and the tower client.
	err = lncfg.Validate(
		cfg.Workers,
		cfg.Caches,
		cfg.WtClient,
		cfg.DB,
		cfg.Cluster,
		cfg.HealthChecks,
		cfg.RPCMiddleware,
		cfg.RemoteSigner,
		cfg.Sweeper,
		cfg.Htlcswitch,
		cfg.Invoices,
		cfg.Routing,
		cfg.Pprof,
		cfg.Gossip,
	)
	if err != nil {
		return nil, err
	}

	// Finally, ensure that the user's color is correctly formatted,
	// otherwise the server will not be able to start after the unlocking
	// the wallet.
	_, err = lncfg.ParseHexColor(cfg.Color)
	if err != nil {
		return nil, mkErr("unable to parse node color: %v", err)
	}

	// All good, return the sanitized result.
	return &cfg, nil
}

// graphDatabaseDir returns the default directory where the local bolt graph db
// files are stored.
func (c *Config) graphDatabaseDir() string {
	return filepath.Join(
		c.DataDir, defaultGraphSubDirname,
		lncfg.NormalizeNetwork(c.ActiveNetParams.Name),
	)
}

// ImplementationConfig returns the configuration of what actual implementations
// should be used when creating the main lnd instance.
func (c *Config) ImplementationConfig(
	interceptor signal.Interceptor) *ImplementationCfg {

	// If we're using a remote signer, we still need the base wallet as a
	// watch-only source of chain and address data. But we don't need any
	// private key material in that btcwallet base wallet.
	if c.RemoteSigner.Enable {
		rpcImpl := NewRPCSignerWalletImpl(
			c, ltndLog, interceptor,
			c.RemoteSigner.MigrateWatchOnly,
		)
		return &ImplementationCfg{
			GrpcRegistrar:     rpcImpl,
			RestRegistrar:     rpcImpl,
			ExternalValidator: rpcImpl,
			DatabaseBuilder: NewDefaultDatabaseBuilder(
				c, ltndLog,
			),
			WalletConfigBuilder: rpcImpl,
			ChainControlBuilder: rpcImpl,
		}
	}

	defaultImpl := NewDefaultWalletImpl(c, ltndLog, interceptor, false)
	return &ImplementationCfg{
		GrpcRegistrar:       defaultImpl,
		RestRegistrar:       defaultImpl,
		ExternalValidator:   defaultImpl,
		DatabaseBuilder:     NewDefaultDatabaseBuilder(c, ltndLog),
		WalletConfigBuilder: defaultImpl,
		ChainControlBuilder: defaultImpl,
	}
}

// CleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
// This function is taken from https://github.com/btcsuite/btcd
func CleanAndExpandPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		var homeDir string
		u, err := user.Current()
		if err == nil {
			homeDir = u.HomeDir
		} else {
			homeDir = os.Getenv("HOME")
		}

		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but the variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

func parseRPCParams(cConfig *lncfg.Chain, nodeConfig interface{},
	netParams chainreg.BitcoinNetParams) error {

	// First, we'll check our node config to make sure the RPC parameters
	// were set correctly. We'll also determine the path to the conf file
	// depending on the backend node.
	var daemonName, confDir, confFile, confFileBase string
	switch conf := nodeConfig.(type) {
	case *lncfg.Btcd:
		// Resolves environment variable references in RPCUser and
		// RPCPass fields.
		conf.RPCUser = supplyEnvValue(conf.RPCUser)
		conf.RPCPass = supplyEnvValue(conf.RPCPass)

		// If both RPCUser and RPCPass are set, we assume those
		// credentials are good to use.
		if conf.RPCUser != "" && conf.RPCPass != "" {
			return nil
		}

		// Set the daemon name for displaying proper errors.
		daemonName = btcdBackendName
		confDir = conf.Dir
		confFileBase = btcdBackendName

		// If only ONE of RPCUser or RPCPass is set, we assume the
		// user did that unintentionally.
		if conf.RPCUser != "" || conf.RPCPass != "" {
			return fmt.Errorf("please set both or neither of "+
				"%[1]v.rpcuser, %[1]v.rpcpass", daemonName)
		}

	case *lncfg.Bitcoind:
		// Ensure that if the ZMQ options are set, that they are not
		// equal.
		if conf.ZMQPubRawBlock != "" && conf.ZMQPubRawTx != "" {
			err := checkZMQOptions(
				conf.ZMQPubRawBlock, conf.ZMQPubRawTx,
			)
			if err != nil {
				return err
			}
		}

		// Ensure that if the estimate mode is set, that it is a legal
		// value.
		if conf.EstimateMode != "" {
			err := checkEstimateMode(conf.EstimateMode)
			if err != nil {
				return err
			}
		}

		// Set the daemon name for displaying proper errors.
		daemonName = bitcoindBackendName
		confDir = conf.Dir
		confFile = conf.ConfigPath
		confFileBase = BitcoinChainName

		// Resolves environment variable references in RPCUser
		// and RPCPass fields.
		conf.RPCUser = supplyEnvValue(conf.RPCUser)
		conf.RPCPass = supplyEnvValue(conf.RPCPass)

		// Check that cookie and credentials don't contradict each
		// other.
		if (conf.RPCUser != "" || conf.RPCPass != "") &&
			conf.RPCCookie != "" {

			return fmt.Errorf("please only provide either "+
				"%[1]v.rpccookie or %[1]v.rpcuser and "+
				"%[1]v.rpcpass", daemonName)
		}

		// We convert the cookie into a user name and password.
		if conf.RPCCookie != "" {
			cookie, err := os.ReadFile(conf.RPCCookie)
			if err != nil {
				return fmt.Errorf("cannot read cookie file: %w",
					err)
			}

			splitCookie := strings.Split(string(cookie), ":")
			if len(splitCookie) != 2 {
				return fmt.Errorf("cookie file has a wrong " +
					"format")
			}
			conf.RPCUser = splitCookie[0]
			conf.RPCPass = splitCookie[1]
		}

		if conf.RPCUser != "" && conf.RPCPass != "" {
			// If all of RPCUser, RPCPass, ZMQBlockHost, and
			// ZMQTxHost are set, we assume those parameters are
			// good to use.
			if conf.ZMQPubRawBlock != "" && conf.ZMQPubRawTx != "" {
				return nil
			}

			// If RPCUser and RPCPass are set and RPCPolling is
			// enabled, we assume the parameters are good to use.
			if conf.RPCPolling {
				return nil
			}
		}

		// If not all of the parameters are set, we'll assume the user
		// did this unintentionally.
		if conf.RPCUser != "" || conf.RPCPass != "" ||
			conf.ZMQPubRawBlock != "" || conf.ZMQPubRawTx != "" {

			return fmt.Errorf("please set %[1]v.rpcuser and "+
				"%[1]v.rpcpass (or %[1]v.rpccookie) together "+
				"with %[1]v.zmqpubrawblock, %[1]v.zmqpubrawtx",
				daemonName)
		}
	}

	// If we're in simnet mode, then the running btcd instance won't read
	// the RPC credentials from the configuration. So if lnd wasn't
	// specified the parameters, then we won't be able to start.
	if cConfig.SimNet {
		return fmt.Errorf("rpcuser and rpcpass must be set to your " +
			"btcd node's RPC parameters for simnet mode")
	}

	fmt.Println("Attempting automatic RPC configuration to " + daemonName)

	if confFile == "" {
		confFile = filepath.Join(confDir, fmt.Sprintf("%v.conf",
			confFileBase))
	}
	switch cConfig.Node {
	case btcdBackendName:
		nConf := nodeConfig.(*lncfg.Btcd)
		rpcUser, rpcPass, err := extractBtcdRPCParams(confFile)
		if err != nil {
			return fmt.Errorf("unable to extract RPC credentials: "+
				"%v, cannot start w/o RPC connection", err)
		}
		nConf.RPCUser, nConf.RPCPass = rpcUser, rpcPass

	case bitcoindBackendName:
		nConf := nodeConfig.(*lncfg.Bitcoind)
		rpcUser, rpcPass, zmqBlockHost, zmqTxHost, err :=
			extractBitcoindRPCParams(netParams.Params.Name,
				nConf.Dir, confFile, nConf.RPCCookie)
		if err != nil {
			return fmt.Errorf("unable to extract RPC credentials: "+
				"%v, cannot start w/o RPC connection", err)
		}
		nConf.RPCUser, nConf.RPCPass = rpcUser, rpcPass
		nConf.ZMQPubRawBlock, nConf.ZMQPubRawTx = zmqBlockHost, zmqTxHost
	}

	fmt.Printf("Automatically obtained %v's RPC credentials\n", daemonName)
	return nil
}

// supplyEnvValue supplies the value of an environment variable from a string.
// It supports the following formats:
// 1) $ENV_VAR
// 2) ${ENV_VAR}
// 3) ${ENV_VAR:-DEFAULT}
//
// Standard environment variable naming conventions:
// - ENV_VAR contains letters, digits, and underscores, and does
// not start with a digit.
// - DEFAULT follows the rule that it can contain any characters except
// whitespace.
//
// Parameters:
// - value: The input string containing references to environment variables
// (if any).
//
// Returns:
// - string: The value of the specified environment variable, the default
// value if provided, or the original input string if no matching variable is
// found or set.
func supplyEnvValue(value string) string {
	// Regex for $ENV_VAR format.
	var reEnvVar = regexp.MustCompile(`^\$([a-zA-Z_][a-zA-Z0-9_]*)$`)

	// Regex for ${ENV_VAR} format.
	var reEnvVarWithBrackets = regexp.MustCompile(
		`^\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}$`,
	)

	// Regex for ${ENV_VAR:-DEFAULT} format.
	var reEnvVarWithDefault = regexp.MustCompile(
		`^\$\{([a-zA-Z_][a-zA-Z0-9_]*):-([\S]+)\}$`,
	)

	// Match against supported formats.
	switch {
	case reEnvVarWithDefault.MatchString(value):
		matches := reEnvVarWithDefault.FindStringSubmatch(value)
		envVariable := matches[1]
		defaultValue := matches[2]
		if envValue := os.Getenv(envVariable); envValue != "" {
			return envValue
		}

		return defaultValue

	case reEnvVarWithBrackets.MatchString(value):
		matches := reEnvVarWithBrackets.FindStringSubmatch(value)
		envVariable := matches[1]
		envValue := os.Getenv(envVariable)

		return envValue

	case reEnvVar.MatchString(value):
		matches := reEnvVar.FindStringSubmatch(value)
		envVariable := matches[1]
		envValue := os.Getenv(envVariable)

		return envValue
	}

	return value
}

// extractBtcdRPCParams attempts to extract the RPC credentials for an existing
// btcd instance. The passed path is expected to be the location of btcd's
// application data directory on the target system.
func extractBtcdRPCParams(btcdConfigPath string) (string, string, error) {
	// First, we'll open up the btcd configuration file found at the target
	// destination.
	btcdConfigFile, err := os.Open(btcdConfigPath)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = btcdConfigFile.Close() }()

	// With the file open extract the contents of the configuration file so
	// we can attempt to locate the RPC credentials.
	configContents, err := io.ReadAll(btcdConfigFile)
	if err != nil {
		return "", "", err
	}

	// Attempt to locate the RPC user using a regular expression. If we
	// don't have a match for our regular expression then we'll exit with
	// an error.
	rpcUserRegexp, err := regexp.Compile(`(?m)^\s*rpcuser\s*=\s*([^\s]+)`)
	if err != nil {
		return "", "", err
	}
	userSubmatches := rpcUserRegexp.FindSubmatch(configContents)
	if userSubmatches == nil {
		return "", "", fmt.Errorf("unable to find rpcuser in config")
	}

	// Similarly, we'll use another regular expression to find the set
	// rpcpass (if any). If we can't find the pass, then we'll exit with an
	// error.
	rpcPassRegexp, err := regexp.Compile(`(?m)^\s*rpcpass\s*=\s*([^\s]+)`)
	if err != nil {
		return "", "", err
	}
	passSubmatches := rpcPassRegexp.FindSubmatch(configContents)
	if passSubmatches == nil {
		return "", "", fmt.Errorf("unable to find rpcuser in config")
	}

	return supplyEnvValue(string(userSubmatches[1])),
		supplyEnvValue(string(passSubmatches[1])), nil
}

// extractBitcoindRPCParams attempts to extract the RPC credentials for an
// existing bitcoind node instance. The routine looks for a cookie first,
// optionally following the datadir configuration option in the bitcoin.conf. If
// it doesn't find one, it looks for rpcuser/rpcpassword.
func extractBitcoindRPCParams(networkName, bitcoindDataDir, bitcoindConfigPath,
	rpcCookiePath string) (string, string, string, string, error) {

	// First, we'll open up the bitcoind configuration file found at the
	// target destination.
	bitcoindConfigFile, err := os.Open(bitcoindConfigPath)
	if err != nil {
		return "", "", "", "", err
	}
	defer func() { _ = bitcoindConfigFile.Close() }()

	// With the file open extract the contents of the configuration file so
	// we can attempt to locate the RPC credentials.
	configContents, err := io.ReadAll(bitcoindConfigFile)
	if err != nil {
		return "", "", "", "", err
	}

	// First, we'll look for the ZMQ hosts providing raw block and raw
	// transaction notifications.
	zmqBlockHostRE, err := regexp.Compile(
		`(?m)^\s*zmqpubrawblock\s*=\s*([^\s]+)`,
	)
	if err != nil {
		return "", "", "", "", err
	}
	zmqBlockHostSubmatches := zmqBlockHostRE.FindSubmatch(configContents)
	if len(zmqBlockHostSubmatches) < 2 {
		return "", "", "", "", fmt.Errorf("unable to find " +
			"zmqpubrawblock in config")
	}
	zmqTxHostRE, err := regexp.Compile(`(?m)^\s*zmqpubrawtx\s*=\s*([^\s]+)`)
	if err != nil {
		return "", "", "", "", err
	}
	zmqTxHostSubmatches := zmqTxHostRE.FindSubmatch(configContents)
	if len(zmqTxHostSubmatches) < 2 {
		return "", "", "", "", errors.New("unable to find zmqpubrawtx " +
			"in config")
	}
	zmqBlockHost := string(zmqBlockHostSubmatches[1])
	zmqTxHost := string(zmqTxHostSubmatches[1])
	if err := checkZMQOptions(zmqBlockHost, zmqTxHost); err != nil {
		return "", "", "", "", err
	}

	// Next, we'll try to find an auth cookie. We need to detect the chain
	// by seeing if one is specified in the configuration file.
	dataDir := filepath.Dir(bitcoindConfigPath)
	if bitcoindDataDir != "" {
		dataDir = bitcoindDataDir
	}
	dataDirRE, err := regexp.Compile(`(?m)^\s*datadir\s*=\s*([^\s]+)`)
	if err != nil {
		return "", "", "", "", err
	}
	dataDirSubmatches := dataDirRE.FindSubmatch(configContents)
	if dataDirSubmatches != nil {
		dataDir = string(dataDirSubmatches[1])
	}

	var chainDir string
	switch networkName {
	case "mainnet":
		chainDir = ""
	case "regtest", "testnet3", "testnet4", "signet":
		chainDir = networkName
	default:
		return "", "", "", "", fmt.Errorf("unexpected networkname %v", networkName)
	}

	cookiePath := filepath.Join(dataDir, chainDir, ".cookie")
	if rpcCookiePath != "" {
		cookiePath = rpcCookiePath
	}
	cookie, err := os.ReadFile(cookiePath)
	if err == nil {
		splitCookie := strings.Split(string(cookie), ":")
		if len(splitCookie) == 2 {
			return splitCookie[0], splitCookie[1], zmqBlockHost,
				zmqTxHost, nil
		}
	}

	// We didn't find a cookie, so we attempt to locate the RPC user using
	// a regular expression. If we  don't have a match for our regular
	// expression then we'll exit with an error.
	rpcUserRegexp, err := regexp.Compile(`(?m)^\s*rpcuser\s*=\s*([^\s]+)`)
	if err != nil {
		return "", "", "", "", err
	}
	userSubmatches := rpcUserRegexp.FindSubmatch(configContents)

	// Similarly, we'll use another regular expression to find the set
	// rpcpass (if any). If we can't find the pass, then we'll exit with an
	// error.
	rpcPassRegexp, err := regexp.Compile(`(?m)^\s*rpcpassword\s*=\s*([^\s]+)`)
	if err != nil {
		return "", "", "", "", err
	}
	passSubmatches := rpcPassRegexp.FindSubmatch(configContents)

	// Exit with an error if the cookie file, is defined in config, and
	// can not be found, with both rpcuser and rpcpassword undefined.
	if rpcCookiePath != "" && userSubmatches == nil && passSubmatches == nil {
		return "", "", "", "", fmt.Errorf("unable to open cookie file (%v)",
			rpcCookiePath)
	}

	if userSubmatches == nil {
		return "", "", "", "", fmt.Errorf("unable to find rpcuser in " +
			"config")
	}
	if passSubmatches == nil {
		return "", "", "", "", fmt.Errorf("unable to find rpcpassword " +
			"in config")
	}

	return supplyEnvValue(string(userSubmatches[1])),
		supplyEnvValue(string(passSubmatches[1])),
		zmqBlockHost, zmqTxHost, nil
}

// checkZMQOptions ensures that the provided addresses to use as the hosts for
// ZMQ rawblock and rawtx notifications are different.
func checkZMQOptions(zmqBlockHost, zmqTxHost string) error {
	if zmqBlockHost == zmqTxHost {
		return errors.New("zmqpubrawblock and zmqpubrawtx must be set " +
			"to different addresses")
	}

	return nil
}

// checkEstimateMode ensures that the provided estimate mode is legal.
func checkEstimateMode(estimateMode string) error {
	for _, mode := range bitcoindEstimateModes {
		if estimateMode == mode {
			return nil
		}
	}

	return fmt.Errorf("estimatemode must be one of the following: %v",
		bitcoindEstimateModes[:])
}

// configToFlatMap converts the given config struct into a flat map of
// key/value pairs using the dot notation we are used to from the config file
// or command line flags. It also returns a map containing deprecated config
// options.
func configToFlatMap(cfg Config) (map[string]string,
	map[string]struct{}, error) {

	result := make(map[string]string)

	// deprecated stores a map of deprecated options found in the config
	// that are set by the users. A config option is considered as
	// deprecated if it has a `hidden` flag.
	deprecated := make(map[string]struct{})

	// redact is the helper function that redacts sensitive values like
	// passwords.
	redact := func(key, value string) string {
		sensitiveKeySuffixes := []string{
			"pass",
			"password",
			"dsn",
		}
		for _, suffix := range sensitiveKeySuffixes {
			if strings.HasSuffix(key, suffix) {
				return "[redacted]"
			}
		}

		return value
	}

	// printConfig is the helper function that goes into nested structs
	// recursively. Because we call it recursively, we need to declare it
	// before we define it.
	var printConfig func(reflect.Value, string)
	printConfig = func(obj reflect.Value, prefix string) {
		// Turn struct pointers into the actual struct, so we can
		// iterate over the fields as we would with a struct value.
		if obj.Kind() == reflect.Ptr {
			obj = obj.Elem()
		}

		// Abort on nil values.
		if !obj.IsValid() {
			return
		}

		// Loop over all fields of the struct and inspect the type.
		for i := 0; i < obj.NumField(); i++ {
			field := obj.Field(i)
			fieldType := obj.Type().Field(i)

			longName := fieldType.Tag.Get("long")
			namespace := fieldType.Tag.Get("namespace")
			group := fieldType.Tag.Get("group")
			hidden := fieldType.Tag.Get("hidden")

			switch {
			// We have a long name defined, this is a config value.
			case longName != "":
				key := longName
				if prefix != "" {
					key = prefix + "." + key
				}

				// Add the value directly to the flattened map.
				result[key] = redact(key, fmt.Sprintf(
					"%v", field.Interface(),
				))

				// If there's a hidden flag, it's deprecated.
				if hidden == "true" && !field.IsZero() {
					deprecated[key] = struct{}{}
				}

			// We have no long name but a namespace, this is a
			// nested struct.
			case longName == "" && namespace != "":
				key := namespace
				if prefix != "" {
					key = prefix + "." + key
				}

				printConfig(field, key)

			// Just a group means this is a dummy struct to house
			// multiple config values, the group name doesn't go
			// into the final field name.
			case longName == "" && group != "":
				printConfig(field, prefix)

			// Anonymous means embedded struct. We need to recurse
			// into it but without adding anything to the prefix.
			case fieldType.Anonymous:
				printConfig(field, prefix)

			default:
				continue
			}
		}
	}

	// Turn the whole config struct into a flat map.
	printConfig(reflect.ValueOf(cfg), "")

	return result, deprecated, nil
}

// logWarningsForDeprecation logs a warning if a deprecated config option is
// set.
func logWarningsForDeprecation(cfg Config) {
	_, deprecated, err := configToFlatMap(cfg)
	if err != nil {
		ltndLog.Errorf("Convert configs to map: %v", err)
	}

	for k := range deprecated {
		ltndLog.Warnf("Config '%s' is deprecated, please remove it", k)
	}
}

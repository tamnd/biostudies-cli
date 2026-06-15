package biostudies

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes BioStudies as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/biostudies-cli/biostudies"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// biostudies:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone biostudies binary (see cli/root.go), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the BioStudies driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "biostudies",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "biostudies",
			Short:  "Read public EMBL-EBI BioStudies research data.",
			Long: `Read public EMBL-EBI BioStudies research data.

biostudies reads from the EMBL-EBI BioStudies repository (3.3M+ records) over
plain HTTPS, shapes it into clean records, and prints output that pipes into
the rest of your tools. No API key required.`,
			Site: "www.ebi.ac.uk/biostudies",
			Repo: "https://github.com/tamnd/biostudies-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// search: keyword search across BioStudies.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search BioStudies and return study records",
		Args:    []kit.Arg{{Name: "query", Help: "search terms"}}}, searchStudies)

	// study: fetch a single study by accession.
	kit.Handle(app, kit.OpMeta{Name: "study", Group: "read", Single: true,
		Summary: "Fetch a study by accession", URIType: "study", Resolver: true,
		Args: []kit.Arg{{Name: "accession", Help: "BioStudies accession (e.g. S-BSST3126)"}}}, getStudy)

	// recent: most recently released studies.
	kit.Handle(app, kit.OpMeta{Name: "recent", Group: "read", List: true,
		Summary: "List recently released BioStudies records"}, recentStudies)
}

// newClient builds the BioStudies client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg" help:"search terms"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Page   int     `kit:"flag" help:"page number (1-based)"`
	Client *Client `kit:"inject"`
}

type studyRef struct {
	Accession string  `kit:"arg" help:"BioStudies accession (e.g. S-BSST3126)"`
	Client    *Client `kit:"inject"`
}

type recentInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Page   int     `kit:"flag" help:"page number (1-based)"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchStudies(ctx context.Context, in searchInput, emit func(*Study) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	page := in.Page
	if page <= 0 {
		page = 1
	}
	studies, _, err := in.Client.Search(ctx, in.Query, limit, page)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range studies {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

func getStudy(ctx context.Context, in studyRef, emit func(*Study) error) error {
	s, err := in.Client.GetStudy(ctx, in.Accession)
	if err != nil {
		return mapErr(err)
	}
	return emit(s)
}

func recentStudies(ctx context.Context, in recentInput, emit func(*Study) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	page := in.Page
	if page <= 0 {
		page = 1
	}
	studies, _, err := in.Client.Search(ctx, "*", limit, page)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range studies {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions, no network ---

// Classify turns any accepted input — a bare accession or a full BioStudies URL —
// into the canonical (type, id). Any non-empty string is accepted as a study id.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty BioStudies reference")
	}
	return "study", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "study" {
		return "", errs.Usage("biostudies has no resource type %q", uriType)
	}
	return "https://www.ebi.ac.uk/biostudies/studies/" + id, nil
}

// --- helpers ---

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}

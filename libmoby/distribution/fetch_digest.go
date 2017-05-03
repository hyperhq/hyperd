package distribution

import (
	"errors"
	"time"

	dockerdist "github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/registry/client"
	mobydist "github.com/docker/docker/distribution"
	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"
	"github.com/docker/engine-api/types"
	"golang.org/x/net/context"
)

func GetDigestFromTag(svc *registry.Service, ref reference.Named, metaHeaders map[string][]string, authConfig *types.AuthConfig) (digest.Digest, error) {
	var (
		ok        bool
		err       error
		tagged    reference.NamedTagged
		ctx       context.Context
		canceller context.CancelFunc
		endpoints []registry.APIEndpoint
		repoInfo  *registry.RepositoryInfo
		dgst      digest.Digest
	)

	tagged, ok = ref.(reference.NamedTagged)
	if !ok {
		return "", errors.New("not a tagged reference")
	}

	endpoints, repoInfo, err = lookupV2Endpoints(svc, ref)
	if err != nil || len(endpoints) == 0 {
		return "", err
	}

	ctx, canceller = context.WithTimeout(context.Background(), 2*time.Minute)
	defer func() {
		if err != nil {
			canceller()
		}
	}()

	for _, endpoint := range endpoints {
		var manSrv dockerdist.ManifestService
		manSrv, err = getSvcWithV2Endpoint(ctx, repoInfo, endpoint, metaHeaders, authConfig)
		if err != nil {
			err = nil
			continue
		}
		dgst, err = getDigest(manSrv, ctx, tagged.Tag())
		if err != nil {
			err = nil
			continue
		}
		return dgst, nil
	}
	return "", errors.New("no available endpoint")
}

func lookupV2Endpoints(svc *registry.Service, ref reference.Named) ([]registry.APIEndpoint, *registry.RepositoryInfo, error) {
	repoInfo, err := svc.ResolveRepository(ref)
	if err != nil {
		return nil, nil, err
	}

	//This method should be called after a successful tagged pull, therefor we do not need to
	//validate the ref name again. In case we want to use this before pull, we should add
	// `validateRepoName()` back here.

	endpoints, err := svc.LookupPullEndpoints(repoInfo)
	if err != nil {
		return nil, nil, err
	}
	res := make([]registry.APIEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Version != registry.APIVersion2 {
			continue
		}
		res = append(res, endpoint)
	}
	return res, repoInfo, nil
}

func getSvcWithV2Endpoint(ctx context.Context, repoInfo *registry.RepositoryInfo, endpoint registry.APIEndpoint, metaHeaders map[string][]string, authConfig *types.AuthConfig) (dockerdist.ManifestService, error) {
	repo, _, err := mobydist.NewV2Repository(ctx, repoInfo, endpoint, metaHeaders, authConfig, "pull")
	if err != nil {
		return nil, err
	}
	manSvc, err := repo.Manifests(ctx)
	if err != nil {
		return nil, err
	}
	return manSvc, nil
}

func getDigest(manSvc dockerdist.ManifestService, ctx context.Context, tag string) (digest.Digest, error) {
	manifest, err := manSvc.Get(ctx, "", client.WithTag(tag))
	if err != nil {
		return "", err
	}
	return manifest2Digist(manifest)
}

func manifest2Digist(mfst dockerdist.Manifest) (digest.Digest, error) {
	switch v := mfst.(type) {
	case *schema1.SignedManifest:
		return digest.FromBytes(v.Canonical), nil
	case *schema2.DeserializedManifest:
		_, canonical, err := v.Payload()
		if err != nil {
			return "", err
		}
		return digest.FromBytes(canonical), nil
	case *manifestlist.DeserializedManifestList: // TODO: I think we should process the mfst list one by one, but let's begin from here.
		_, canonical, err := v.Payload()
		if err != nil {
			return "", err
		}
		return digest.FromBytes(canonical), nil
	default:
		return "", errors.New("unsupported manifest format")
	}
}

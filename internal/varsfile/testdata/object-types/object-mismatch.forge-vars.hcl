# repo_type declared as string in blueprint, but supplied as a list —
# a shape cty.Convert can't coerce to string.
git_provider = {
  repo_type   = ["github"]
  repo_url    = "github.com"
  project_org = "donaldgifford"
}

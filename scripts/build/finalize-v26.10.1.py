#!/usr/bin/env python3
"""One-time, user-authorized retirement of the four v26.10.0 RC releases.

Runs only after both platform publication jobs succeed. Never reads local Git
credentials; gh uses the workflow's existing, ephemeral contents-write token.
"""
import json
import os
from pathlib import Path
import subprocess

REPOSITORY = 'yuhongwei380/DevShell'
STABLE_TAG = 'v26.10.1-build20261023150'
RC_TAGS = (
    'v26.10.0-rc.1-build20261023146',
    'v26.10.0-rc.2-build20261023147',
    'v26.10.0-rc.3-build20261023148',
    'v26.10.0-rc.4-build20261023149',
)
REQUIRED_ASSETS = {
    'DevShell-Setup-x64.exe', 'up.exe', 'up.exe.json',
    'DevShell-macos-universal.dmg', 'DevShell-macos-universal.zip',
    'up-macos-arm64.json', 'up-macos-amd64.json',
    'SHA256SUMS.txt', 'build-info.json',
}


def gh(*args):
    result = subprocess.run(['gh', *args], text=True, capture_output=True, check=True)
    return result.stdout


def api(path):
    return json.loads(gh('api', f'repos/{REPOSITORY}/{path}'))


def releases():
    pages = json.loads(gh('api', '--paginate', '--slurp',
                          f'repos/{REPOSITORY}/releases?per_page=100'))
    return [release for page in pages for release in page]


def validate(stable, inventory):
    if (stable.get('tagName') != STABLE_TAG or stable.get('isDraft') is not False
            or stable.get('isPrerelease') is not False):
        raise ValueError('Expected the published v26.10.1 stable release')
    assets = {asset['name']: asset for asset in stable.get('assets', [])}
    if not REQUIRED_ASSETS.issubset(assets):
        raise ValueError(f'Missing stable assets: {sorted(REQUIRED_ASSETS - assets.keys())}')
    if any(assets[name].get('size', 0) <= 0 for name in REQUIRED_ASSETS):
        raise ValueError('Stable release contains an empty asset')
    # Validate the entire deletion scope before changing any release or tag.
    for release in inventory:
        if release.get('tag_name') in RC_TAGS:
            if release.get('prerelease') is not True or release.get('draft') is not False:
                raise ValueError(f'Refusing to remove a non-RC release: {release["tag_name"]}')


def main():
    if os.environ.get('GH_REPO') != REPOSITORY:
        raise ValueError('Unexpected repository; no changes made')
    stable = json.loads(gh('release', 'view', STABLE_TAG, '--json',
                           'tagName,isDraft,isPrerelease,assets'))
    inventory = releases()
    validate(stable, inventory)
    gh('release', 'edit', STABLE_TAG, '--latest')
    present = {release['tag_name'] for release in inventory}
    for tag in RC_TAGS:
        if tag in present:
            gh('release', 'delete', tag, '--yes')
        # Separate deletion also makes a rerun recover from a partial cleanup.
        refs = api(f'git/matching-refs/tags/{tag}')
        if any(ref.get('ref') == f'refs/tags/{tag}' for ref in refs):
            gh('api', '--method', 'DELETE', f'repos/{REPOSITORY}/git/refs/tags/{tag}')
        print(f'Retired {tag}', flush=True)
    remaining = {release['tag_name'] for release in releases()}
    refs = api('git/matching-refs/tags/v26.10.0-rc.')
    if any(tag in remaining for tag in RC_TAGS) or any(
            ref.get('ref') in {f'refs/tags/{tag}' for tag in RC_TAGS} for ref in refs):
        raise ValueError('RC cleanup verification failed')
    if api('releases/latest').get('tag_name') != STABLE_TAG:
        raise ValueError('Stable release is not Latest')
    summary = os.environ.get('GITHUB_STEP_SUMMARY')
    if summary:
        with Path(summary).open('a', encoding='utf-8') as output:
            output.write(f'Published **{STABLE_TAG}** with all platform assets.\n\n')
            output.write('Removed RC releases and tags:\n\n' + ''.join(f'- {tag}\n' for tag in RC_TAGS))


if __name__ == '__main__':
    main()

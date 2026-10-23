"""Offline checks for the destructive release cleanup's fail-closed guards."""
import copy
import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    'finalize_release', Path(__file__).resolve().parents[1] / 'build' / 'finalize-v26.10.1.py')
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class FinalizeReleaseTests(unittest.TestCase):
    def setUp(self):
        self.stable = {
            'tagName': release.STABLE_TAG, 'isDraft': False, 'isPrerelease': False,
            'assets': [{'name': name, 'size': 123} for name in release.REQUIRED_ASSETS],
        }

    def test_complete_stable_and_only_the_named_rc_releases_are_allowed(self):
        release.validate(self.stable, [
            {'tag_name': tag, 'prerelease': True, 'draft': False} for tag in release.RC_TAGS
        ] + [{'tag_name': 'v26.10.0-build20261023145', 'prerelease': False}])

    def test_missing_or_empty_platform_assets_stop_cleanup(self):
        for name in release.REQUIRED_ASSETS:
            for empty in (False, True):
                candidate = copy.deepcopy(self.stable)
                if empty:
                    next(asset for asset in candidate['assets'] if asset['name'] == name)['size'] = 0
                else:
                    candidate['assets'] = [asset for asset in candidate['assets'] if asset['name'] != name]
                with self.subTest(name=name, empty=empty), self.assertRaises(ValueError):
                    release.validate(candidate, [])

    def test_wrong_tag_draft_and_prerelease_stop_cleanup(self):
        for key, value in [('tagName', 'other-tag'), ('isDraft', True), ('isPrerelease', True)]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                release.validate({**self.stable, key: value}, [])

    def test_rc_that_was_promoted_to_stable_cannot_be_deleted(self):
        with self.assertRaises(ValueError):
            release.validate(self.stable, [{'tag_name': release.RC_TAGS[0], 'prerelease': False, 'draft': False}])

    def test_wrong_repository_never_invokes_gh(self):
        with patch.dict('os.environ', {'GH_REPO': 'other/repository'}), patch.object(release, 'gh') as gh:
            with self.assertRaises(ValueError):
                release.main()
            gh.assert_not_called()

    def test_incomplete_stable_never_mutates_releases_or_tags(self):
        incomplete = {**self.stable, 'assets': self.stable['assets'][:-1]}
        with patch.dict('os.environ', {'GH_REPO': release.REPOSITORY}), \
                patch.object(release, 'gh', return_value=json.dumps(incomplete)) as gh, \
                patch.object(release, 'releases', return_value=[]):
            with self.assertRaises(ValueError):
                release.main()
            self.assertEqual(gh.call_count, 1)
            self.assertEqual(gh.call_args.args[:2], ('release', 'view'))


if __name__ == '__main__':
    unittest.main()

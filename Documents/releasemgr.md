
## Release Manager Design
### Requests
- Versions are composed of three digits strings: major.minor.patch. For example, 1.2.3 means major version 1, minor version 2, and patch version 3.
- A new release can be requested by any developer. The request should contain the target version, a short description of the release, a release note document (markdown) and any other relevant information.
- The release request should be sent to the release manager (a designated person or team responsible for managing releases).
### Release Process
- The release manager reviews the release request and approves or rejects it based on the project's release policy.
- Once approved, the release manager coordinates with the development team to prepare the release.
- The development team should ensure that all code changes for the release are merged into the main branch and that the codebase is stable.
- The release manager should create a release branch from the main branch for the release.
- The release manager should tag the release branch with the target version.
- The release manager should build the release artifacts (e.g., binaries, packages) from the release branch.
### Post-Release    
- We will create a release note in $project-name/docs/releases. 
- Release information should contain: versions, the release note document name, the release date and time, the developer's name responsible for the release, a short description about the release, and some other information 
- When the system starts, the main should print the release information
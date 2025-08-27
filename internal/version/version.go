// Package version provides centralized version information for Prism monorepo projects.
// This package supports independent versioning for prismd daemon and prismctl CLI
// as separate projects within the monorepo, allowing them to evolve independently
// while maintaining consistency within each project's components.
// All versions follow semantic versioning (semver) conventions.

package version

// PrismdVersion holds the current prismd daemon version.
// Format: major.minor.patch[-prerelease][+build]
const PrismdVersion = "0.1.0-dev"

// PrismctlVersion holds the current prismctl CLI version.
// This is used by the CLI binary and allows independent evolution
// of the management tool separate from the daemon infrastructure.
// Format: major.minor.patch[-prerelease][+build]
const PrismctlVersion = "0.1.0-dev"

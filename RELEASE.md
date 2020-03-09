# Release

The kubernetes-sre-addons repository is intended to support a set of Addons required to provide *secured cloud-provider operations* including monitoring, logging, alerting, and backups for every version of Kubernetes actively supported by the upstream Kubernetes community.
It is not intended to be tied to any specific version of Kubernetes installer (ie. Konvoy or Kommander), or Kubeaddons.
Changes to Konvoy or Kommander to create a resource needed by an Addon should be avoided.

- [Schedule](#schedule)
- [Considerations](#considerations)
  - [Kubernetes](#kubernetes)
  - [Branches and Tags](#branches-and-tags)
- [Process](#process)
  - [Testing Release (Weekly, Thursday)](#testing-release-weekly-thursday)
  - [Stable Release (Weekly, Wednesday)](#stable-release-weekly-wednesday)

## Schedule

Releases should be bi-weekly<sup>[1](#footnote1)</sup> on Wednesdays, or as needed to address CVEs.

## Considerations

### Kubernetes

The Addons in this repo are the minimum base set of supported<sup>[2](#footnote2)</sup> Addons to be installed as a suite for any supported version of Kubernetes.

Kubernetes support must be maintained for the latest general release of Kubernetes and the two prior minor releases.
At the time of this writing, the latest Kubernetes release is 1.17.2.
Support from this repo, at this time, should cover 1.15.x, 1.16.x, and 1.17.x.

### Branches and Tags

As much as possible, we will try to maintain a `master` branch that is compatible with all supported versions of Kubernetes.
If there is a variation in the Kubernetes API which requires a _**breaking change**_ to the Addon, a _**branch**_ will be made for the prior Kubernetes versions.
All future changes adopted into master will need to be back-ported to those branches.

**NOTE**: No other changes may be breaking. Extreme changes, like moving from traefik 1.7 to 2.2, must be done in such a way that the transition is transparent to the user.

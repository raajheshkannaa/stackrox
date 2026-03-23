FROM registry.access.redhat.com/ubi8:latest AS ubi-base
FROM registry.access.redhat.com/ubi8/ubi-micro:latest AS ubi-micro-base

FROM ubi-base AS mappings

COPY download-mappings.sh /download-mappings.sh
RUN /download-mappings.sh /mappings

FROM ubi-base AS package_installer

# Copy ubi-micro base to preserve rpmdb
COPY --from=ubi-micro-base / /out/

# Install packages directly to /out/ using --installroot
RUN dnf install \
    --installroot=/out/ \
    --releasever=8 \
    --setopt=install_weak_deps=0 \
    --nodocs \
    -y \
    findutils \
    util-linux \
    ca-certificates && \
    dnf --installroot=/out/ clean all && \
    rm -rf /out/var/cache/dnf /out/var/cache/yum

COPY scripts/save-dir-contents /out/usr/local/bin/

# The contents of paths mounted as emptyDir volumes in Kubernetes are saved
# by the script `save-dir-contents` during the image build. The directory
# contents are then restored by the script `restore-all-dir-contents`
# during the container start.
# Setup CA trust directories with proper permissions in /out/
RUN chown -R 65534:65534 /out/etc/pki/ca-trust /out/etc/ssl /out/tmp && \
    chroot /out /usr/local/bin/save-dir-contents /etc/pki/ca-trust /etc/ssl

FROM ubi-micro-base

ARG LABEL_VERSION
ARG LABEL_RELEASE
ARG QUAY_TAG_EXPIRATION

LABEL name="scanner-v4" \
      vendor="StackRox" \
      maintainer="https://stackrox.io/" \
      summary="Static vulnerability scanner for the StackRox Security Platform" \
      description="This image supports static vulnerability scanning for the StackRox Security Platform." \
      version="${LABEL_VERSION}" \
      release="${LABEL_RELEASE}" \
      quay.expires-after="${QUAY_TAG_EXPIRATION}"

COPY --from=package_installer /out/ /

SHELL ["/bin/sh", "-o", "pipefail", "-c"]

COPY scripts/entrypoint.sh \
     scripts/import-additional-cas \
     scripts/restore-all-dir-contents \
     scripts/save-dir-contents /usr/local/bin/
COPY bin/scanner /usr/local/bin/
COPY THIRD_PARTY_NOTICES/ /THIRD_PARTY_NOTICES/

COPY --from=mappings /mappings/repository-to-cpe.json /mappings/container-name-repos-map.json /run/mappings/

# This is equivalent to nobody:nobody.
USER 65534:65534

ENTRYPOINT ["entrypoint.sh"]

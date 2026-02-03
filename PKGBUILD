# Maintainer: Gary <apollosenvy@users.noreply.github.com>
pkgname=roundtable
pkgver=0.1.0
pkgrel=1
pkgdesc="Multi-model debate TUI for complex decisions"
arch=('x86_64')
url="https://github.com/apollosenvy/roundtable"
license=('AGPL-3.0-or-later')
depends=('glibc')
makedepends=('go>=1.22')
optdepends=(
    'claude-code: Claude CLI for primary model'
    'gemini-cli: Gemini CLI support'
)
source=("${pkgname}-${pkgver}.tar.gz::https://github.com/apollosenvy/roundtable/archive/v${pkgver}.tar.gz")
sha256sums=('SKIP')

build() {
    cd "${pkgname}-${pkgver}"
    export CGO_CPPFLAGS="${CPPFLAGS}"
    export CGO_CFLAGS="${CFLAGS}"
    export CGO_CXXFLAGS="${CXXFLAGS}"
    export CGO_LDFLAGS="${LDFLAGS}"
    export GOFLAGS="-buildmode=pie -trimpath -ldflags=-linkmode=external -mod=readonly -modcacherw"
    go build -ldflags "-X main.Version=${pkgver}" -o roundtable ./cmd/roundtable
}

package() {
    cd "${pkgname}-${pkgver}"
    install -Dm755 roundtable "${pkgdir}/usr/bin/roundtable"
    install -Dm644 LICENSE "${pkgdir}/usr/share/licenses/${pkgname}/LICENSE"
    install -Dm644 README.md "${pkgdir}/usr/share/doc/${pkgname}/README.md"
}

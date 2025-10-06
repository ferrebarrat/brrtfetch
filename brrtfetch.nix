{ lib, stdenv, fetchFromGitHub, go }:

stdenv.mkDerivation rec {
  pname = "brrtfetch";
  version = "unstable-2025-10-05";

  src = fetchFromGitHub {
    owner = "ferrebarrat";
    repo = "brrtfetch";
    rev = "main";
    hash = "sha256-hC2R17LadoVanjLs4iBTr55qNJEDYKI9H3lQzyHQIek=";
  };

  nativeBuildInputs = [ go ];

  buildPhase = ''
    export GOCACHE=$TMPDIR/go-cache
    go build -o brrtfetch ./go/main.go
  '';

  installPhase = ''
    mkdir -p $out/bin
    cp brrtfetch $out/bin/
  '';

  meta = {
    description = "Render animated ASCII art from a GIF for your sysinfo fetcher of choice";
    homepage = "https://github.com/ferrebarrat/brrtfetch";
    license = lib.licenses.mit;
    mainProgram = "brrtfetch";
    maintainers = with lib.maintainers; [ ];
    platforms = lib.platforms.all;
  };
}

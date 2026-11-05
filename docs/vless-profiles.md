# Dua profil VLESS pada satu instance Xray

Panel mempertahankan inbound utama **VLESS + TCP/RAW + REALITY + Vision** dan dapat membuat satu inbound tambahan **VLESS + XHTTP + REALITY**. Kedua inbound menggunakan port internal, pengaturan transport, path, serta key/short ID REALITY masing-masing. Pengguna yang sama dapat memilih link TCP Vision atau link XHTTP; flow Vision hanya dipakai pada inbound TCP.

## Dukungan dan batas konfigurasi

Dokumentasi Xray menyatakan REALITY hanya dipakai bersama transport RAW, XHTTP, dan gRPC; XHTTP secara eksplisit termasuk transport yang kompatibel. Lihat [REALITY](https://xtls.github.io/en/config/transports/reality.html) dan [matriks transport Xray](https://xtls.github.io/en/config/transport.html). Dokumentasi [VLESS](https://xtls.github.io/en/config/inbounds/vless.html) membatasi `xtls-rprx-vision` pada kombinasi TCP/RAW + TLS/REALITY (atau skenario VLESS Encryption yang tidak dipakai panel ini). Karena itu, config inbound TCP utama memiliki flow Vision, sedangkan inbound XHTTP tidak.

Setiap inbound Xray mempunyai objek listener, protocol settings, dan `streamSettings` sendiri dalam array `inbounds`, sebagaimana dijelaskan pada [Inbound Proxy](https://xtls.github.io/en/config/inbound.html). Panel menggunakan struktur ini untuk mengisolasi port, path XHTTP, key, short ID, dan mapping provider tanpa mengubah listener utama.

## Port provider

Panel tidak mengarang alamat publik. Setelah provider menetapkan endpoint untuk inbound XHTTP, masukkan hostname:port pada kartu inbound tersebut. Arahkan endpoint itu melalui **TCP** ke port internal yang ditampilkan panel. Endpoint utama tetap diarahkan ke port internal inbound utama. Maksimal delapan endpoint Xray unik total dipakai bersama oleh kedua inbound; satu slot dari batas sembilan mapping disediakan untuk panel.

Uji `xray run -test -config` memeriksa validitas file konfigurasi, tetapi tidak menjalankan listener dan bukan bukti bahwa mapping provider atau konektivitas klien eksternal berhasil. Setelah mapping tersedia, uji koneksi klien dari luar VPS secara terpisah.

## Kunci, tautan, dan backup

Profil kedua menghasilkan keypair REALITY, short ID, serta path acak yang berbeda dari profil utama. Client URI hanya menerima material sisi klien; private key server tidak boleh muncul pada API state, link/QR, maupun log. State dan backup berisi private key server, sehingga backup harus disimpan sebagai rahasia terenkripsi.

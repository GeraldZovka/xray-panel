# Inbound multi-profil pada satu instance Xray

Panel mempertahankan inbound utama **VLESS + TCP/RAW + REALITY + Vision** dan mendukung banyak inbound tambahan yang berjalan bersamaan. Profil tambahan dapat berupa VLESS TCP + REALITY + Vision, VLESS XHTTP + REALITY, VMess, atau Shadowsocks. Setiap inbound punya ID, nama, port internal, mapping publik, dan status tersendiri. Profil VLESS tambahan menggunakan key REALITY sendiri; profil XHTTP juga memakai path sendiri.

## Transport dan konfigurasi gabungan

Dokumentasi Xray menyatakan REALITY kompatibel dengan transport RAW, XHTTP, dan gRPC; XHTTP termasuk transport yang kompatibel. Lihat [REALITY](https://xtls.github.io/en/config/transports/reality.html) dan [matriks transport Xray](https://xtls.github.io/en/config/transport.html). [VLESS](https://xtls.github.io/en/config/inbounds/vless.html) membatasi `xtls-rprx-vision` pada kombinasi TCP/RAW + TLS/REALITY; karena itu flow Vision hanya dipasang pada profil VLESS TCP, tidak pada XHTTP. VMess dan Shadowsocks yang dikelola panel memakai transport TCP.

Setiap inbound mempunyai listener, protocol settings, dan `streamSettings` sendiri dalam array `inbounds`, sebagaimana dijelaskan pada [Inbound Proxy](https://xtls.github.io/en/config/inbound.html). Panel memvalidasi dan menguji config gabungan dengan `xray run -test -config` sebelum menerapkan perubahan. Bila validasi atau restart gagal, helper memulihkan config sebelumnya. Aksi Jalankan/Hentikan mengaktifkan atau menghapus hanya listener profil terkait dari config; Xray dapat restart sebentar agar perubahan berlaku. Minimal satu inbound harus tetap aktif dan profil utama tidak dapat dihapus.

## Port provider

Panel tidak mengarang alamat publik. Setelah provider menetapkan endpoint, masukkan `hostname:port` pada kartu inbound yang tepat di Dashboard. Arahkan endpoint publik melalui **TCP** ke port internal inbound tersebut. Maksimal delapan endpoint Xray unik dipakai bersama semua inbound; satu slot dari batas sembilan mapping total disediakan untuk panel. Port publik dapat berbeda dari port internal.

Uji `xray run -test -config` memeriksa file konfigurasi, tetapi tidak menjalankan listener dan bukan bukti bahwa mapping provider atau konektivitas klien eksternal berhasil. Setelah mapping tersedia, uji koneksi klien dari luar VPS secara terpisah.

## Kunci, tautan, dan backup

Tiap pengguna mendapatkan tautan/QR per profil dengan endpoint publik yang lengkap. Profil yang dinonaktifkan tidak menampilkan tautan aktif. Client URI hanya memuat material sisi klien; private key server tidak boleh muncul pada API state, link/QR, maupun log. State dan file backup menyimpan key server, sehingga backup harus diperlakukan sebagai rahasia dan disimpan aman.

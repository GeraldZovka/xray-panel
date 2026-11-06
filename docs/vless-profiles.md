# Inbound multi-profil pada satu instance Xray

Secara bawaan panel membuat inbound utama **VLESS + TCP/RAW + REALITY + Vision** dan mendukung banyak inbound tambahan yang berjalan bersamaan. Profil tambahan dapat berupa VLESS TCP + REALITY + Vision, VLESS XHTTP + REALITY, VMess, atau Shadowsocks. Setiap inbound punya ID, nama, port internal, mapping publik, dan status tersendiri. Profil VLESS tambahan menggunakan key REALITY sendiri; profil XHTTP juga memakai path sendiri. Profil utama dapat dihapus dari Dashboard jika tidak diperlukan.

## Transport dan konfigurasi gabungan

Dokumentasi Xray menyatakan REALITY kompatibel dengan transport RAW, XHTTP, dan gRPC; XHTTP termasuk transport yang kompatibel. Lihat [REALITY](https://xtls.github.io/en/config/transports/reality.html) dan [matriks transport Xray](https://xtls.github.io/en/config/transport.html). [VLESS](https://xtls.github.io/en/config/inbounds/vless.html) membatasi `xtls-rprx-vision` pada kombinasi TCP/RAW + TLS/REALITY; karena itu flow Vision hanya dipasang pada profil VLESS TCP, tidak pada XHTTP. VMess dan Shadowsocks yang dikelola panel memakai transport TCP.

Setiap inbound mempunyai listener, protocol settings, dan `streamSettings` sendiri dalam array `inbounds`, sebagaimana dijelaskan pada [Inbound Proxy](https://xtls.github.io/en/config/inbound.html). Panel memvalidasi dan menguji config gabungan dengan `xray run -test -config` sebelum menerapkan perubahan. Bila validasi atau restart gagal, helper memulihkan config sebelumnya. Aksi Start/Stop dan Hapus berlaku hanya pada profil terkait; Xray dapat restart singkat agar perubahan berlaku. Semua profil boleh berhenti atau dihapus—daemon Xray tetap berjalan dengan array `inbounds` kosong, dan profil baru dapat ditambahkan kemudian.

## Port provider

Panel tidak mengarang alamat publik. Setelah provider menetapkan endpoint, masukkan `hostname:port` pada kartu inbound yang tepat di Dashboard. Arahkan endpoint publik melalui **TCP** ke port internal inbound tersebut. Maksimal delapan endpoint Xray unik dipakai bersama semua inbound; satu slot dari batas sembilan mapping total disediakan untuk panel. Port publik dapat berbeda dari port internal.

Uji `xray run -test -config` memeriksa file konfigurasi, tetapi tidak menjalankan listener dan bukan bukti bahwa mapping provider atau konektivitas klien eksternal berhasil. Setelah mapping tersedia, uji koneksi klien dari luar VPS secara terpisah.

## Kunci, tautan, dan backup

Saat menambah pengguna, admin memilih satu profil/protokol aktif yang akan diberikan. Pengguna lama tetap mempertahankan akses sebelumnya. Tautan/QR hanya ditampilkan untuk profil yang ditetapkan, aktif, dan memiliki endpoint publik lengkap. Client URI hanya memuat material sisi klien; private key server tidak boleh muncul pada API state, link/QR, maupun log. State dan file backup menyimpan key server, sehingga backup harus diperlakukan sebagai rahasia dan disimpan aman.

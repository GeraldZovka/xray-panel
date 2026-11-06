# Inbound multi-profil pada satu instance Xray

Secara bawaan panel membuat inbound utama **VLESS + TCP/RAW + REALITY + Vision** dan mendukung banyak inbound tambahan yang berjalan bersamaan. Profil tambahan dapat berupa VLESS TCP + REALITY + Vision, VLESS XHTTP + REALITY, VMess, atau Shadowsocks. Setiap inbound punya ID, nama, port internal, endpoint publik, dan status tersendiri. Profil VLESS tambahan menggunakan kunci REALITY sendiri; profil XHTTP juga memakai path sendiri. Profil utama dapat dihapus dari Dashboard jika tidak diperlukan.

## Transport dan konfigurasi gabungan

Dokumentasi Xray menyatakan REALITY kompatibel dengan transport RAW, XHTTP, dan gRPC; XHTTP termasuk transport yang kompatibel. Lihat [REALITY](https://xtls.github.io/en/config/transports/reality.html) dan [matriks transport Xray](https://xtls.github.io/en/config/transport.html). [VLESS](https://xtls.github.io/en/config/inbounds/vless.html) membatasi `xtls-rprx-vision` pada kombinasi TCP/RAW + TLS/REALITY; karena itu flow Vision hanya dipasang pada profil VLESS TCP, tidak pada XHTTP. VMess dan Shadowsocks yang dikelola panel memakai transport TCP.

Setiap inbound mempunyai listener, pengaturan protokol, dan `streamSettings` sendiri dalam array `inbounds`, sebagaimana dijelaskan pada [Inbound Proxy](https://xtls.github.io/en/config/inbound.html). Panel memvalidasi dan menguji konfigurasi gabungan dengan `xray run -test -config` sebelum menerapkan perubahan. Bila validasi atau restart gagal, program bantu memulihkan konfigurasi sebelumnya. Tombol Mulai, Hentikan, dan Hapus hanya berlaku pada profil terkait; Xray mungkin perlu dimulai ulang sebentar agar perubahan berlaku. Semua profil boleh dihentikan atau dihapus—daemon Xray tetap berjalan dengan array `inbounds` kosong, dan profil baru dapat ditambahkan kemudian.

## Endpoint publik dan port

Masukkan endpoint publik dalam format `host:port` pada kartu inbound yang sesuai di Dashboard. Endpoint tersebut dipakai untuk membuat tautan dan QR bagi profil. Pastikan pengaturan jaringan Anda dapat meneruskan koneksi ke port internal inbound tersebut; port endpoint dapat berbeda dari port internal.

Uji `xray run -test -config` memeriksa file konfigurasi, tetapi tidak menjalankan listener dan bukan bukti bahwa endpoint dapat dijangkau dari luar. Uji koneksi klien dari jaringan eksternal secara terpisah.

## Kunci, tautan, dan cadangan

Saat menambah pengguna, admin memilih satu profil/protokol aktif yang akan diberikan. Pengguna lama tetap mempertahankan akses sebelumnya. Tautan/QR hanya ditampilkan untuk profil yang ditetapkan, aktif, dan memiliki endpoint publik lengkap. URI klien hanya memuat material sisi klien; kunci privat server tidak boleh muncul pada API status, tautan/QR, maupun log. Data panel dan berkas cadangan menyimpan kunci server, sehingga cadangan harus diperlakukan sebagai rahasia dan disimpan dengan aman.

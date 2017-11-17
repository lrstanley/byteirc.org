$(function () {
    $(".linkify").linkify({ target: "_blank" });
    $('.wants-popup').popup({
        inline: true,
        on: 'hover',
        exclusive: true,
        hoverable: true,
    });
});

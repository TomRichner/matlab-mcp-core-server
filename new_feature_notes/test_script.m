fs = 100;
f = 1;
t = 0:1/fs:2;
y = sin(2*pi*f*t);

figure(1)
plot(t, y)